package e2e

import (
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/appscode/errors"
	aci "github.com/appscode/k8s-addons/api"
	"github.com/appscode/log"
	"github.com/appscode/voyager/pkg/controller/ingress"
	"github.com/appscode/voyager/test/test-server/testserverclient"
	"k8s.io/kubernetes/pkg/api"
	k8serr "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/intstr"
)

const maxRetries = 50

var TestNamespace = "default"

var (
	defaultUrlTemplate = template.Must(template.New("svc-template").Parse("http://{{.IP}}:{{.Port}}"))
)

func (i *IngressTestSuit) TestIngressEnsureTPR() error {
	var err error
	for it := 0; it < 10; it++ {
		log.Infoln(it, "Trying to get ingress.appscode.com")
		tpr, err := i.t.KubeClient.Extensions().ThirdPartyResources().Get("ingress.appscode.com")
		if err == nil {
			log.Infoln("Found tpr for ingress with name", tpr.Name)
			break
		}
		err = errors.New().WithCause(err).Err()
		time.Sleep(time.Second * 5)
		continue
	}
	return err
}

func (ing *IngressTestSuit) TestIngressCreate() error {
	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	var svc *api.Service
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}
	log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
	for i := 0; i < maxRetries; i++ {
		_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for endpoints to be created")
	}
	if err != nil {
		return err
	}

	serverAddr, err := ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 30)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(100)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}

	return nil
}

func (ing *IngressTestSuit) TestIngressDaemonCreate() error {
	if !ing.t.Config.InCluster && ing.t.Config.ProviderName != "minikube" {
		log.Infoln("Test is Running from outside of cluster skipping test")
		return nil
	}

	var nodeSelector = func() string {
		if ing.t.Config.ProviderName == "minikube" {
			return "kubernetes.io/hostname=minikube"
		} else {
			if len(ing.t.Config.DaemonHostName) > 0 {
				return "kubernetes.io/hostname=" + ing.t.Config.DaemonHostName
			}
			return "kubernetes.io/hostname=" + ing.t.Config.ClusterName + "-master"
		}
		return ""
	}

	baseDaemonIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
			Annotations: map[string]string{
				ingress.LBType:             ingress.LBHostPort,
				ingress.DaemonNodeSelector: nodeSelector(),
			},
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseDaemonIngress.Namespace).Create(baseDaemonIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseDaemonIngress.Namespace).Delete(baseDaemonIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	for i := 0; i < maxRetries; i++ {
		_, err := ing.t.KubeClient.Core().Services(baseDaemonIngress.Namespace).Get(ingress.VoyagerPrefix + baseDaemonIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return errors.New().WithCause(err).Err()
	}

	serverAddr, err := ing.getDaemonURLs(baseDaemonIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 20)
	log.Infoln("Loadbalancer created, calling http endpoints for test, Total url found", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressDelete() error {
	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	var svc *api.Service
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}
	log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
	for i := 0; i < maxRetries; i++ {
		_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for endpoints to be created")
	}
	if err != nil {
		return err
	}

	serverAddr, err := ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 20)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(5)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}

	err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
	if err != nil {
		return errors.New().WithCause(err).WithMessage("Failed to delete").Err()
	}

	// Wait until everything is deleted
	time.Sleep(time.Second * 10)
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err != nil {
			if k8serr.IsNotFound(err) {
				break
			}
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be Deleted")
	}

	newServerAddr, err := ing.getURLs(baseIngress)
	if err == nil {
		if len(newServerAddr) != 0 {
			return errors.New().WithMessage("Server address found").Err()
		}
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressUpdate() error {
	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	var svc *api.Service
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}
	log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
	for i := 0; i < maxRetries; i++ {
		_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for endpoints to be created")
	}
	if err != nil {
		return err
	}

	serverAddr, err := ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 20)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}

	updatedBaseIngress, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Get(baseIngress.Name)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}
	updatedBaseIngress.Spec.Rules[0].HTTP.Paths[0].Path = "/newTestpath"
	_, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Update(updatedBaseIngress)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}

	time.Sleep(time.Second * 30)
	serverAddr, err = ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 30)
	log.Infoln("Loadbalancer created, calling http endpoints for updated path, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(1)
		if err == nil {
			return errors.New().WithCause(err).WithMessage("Connected with old prefix").Err()
		}
		log.Infoln("Expected exception, faild to connect with old path, calling new paths.")
		resp, err = testserverclient.NewTestHTTPClient(url).Method("GET").Path("/newTestpath/ok").DoWithRetry(5)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to Connect With New Prefix").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/newTestpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}

	// Open New Port
	updatedBaseIngress, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Get(baseIngress.Name)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}

	if ing.t.Config.ProviderName != "minikube" {
		updatedBaseIngress.Spec.Rules[0].HTTP = nil
		updatedBaseIngress.Spec.Rules[0].TCP = []aci.TCPExtendedIngressRuleValue{
			{
				Port: intstr.FromString("4545"),
				Backend: aci.IngressBackend{
					ServiceName: testServerSvc.Name,
					ServicePort: intstr.FromString("4545"),
				},
			},
			{
				Port: intstr.FromString("4949"),
				Backend: aci.IngressBackend{
					ServiceName: testServerSvc.Name,
					ServicePort: intstr.FromString("4545"),
				},
			},
		}
		_, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Update(updatedBaseIngress)
		if err != nil {
			return errors.New().WithCause(err).Err()
		}
		time.Sleep(time.Second * 30)

		found := false
		for i := 1; i <= maxRetries; i++ {
			svc, err := ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
			if err != nil {
				continue
			}
			log.Infoln("Got Service", svc.Name)
			for _, port := range svc.Spec.Ports {
				log.Infoln(port)
				if port.Port == 4545 {
					found = true
					break
				}
			}
			if found {
				break
			}
			time.Sleep(time.Second * 5)
		}

		if !found {
			return errors.New().WithMessage("Service not found").Err()
		}

		serverAddr, err = ing.getURLs(baseIngress)
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 30)
		log.Infoln("Loadbalancer created, calling http endpoints for updated path, Total", len(serverAddr))
		for _, url := range serverAddr {
			resp, err := testserverclient.NewTestTCPClient(url).DoWithRetry(50)
			if err != nil {
				return errors.New().WithCause(err).WithMessage("Failed to Connect With New Prefix").Err()
			}
			log.Infoln("Response", *resp)
			if resp.ServerPort != ":4545" {
				return errors.New().WithMessage("Port did not matched").Err()
			}
		}

		log.Infoln("Checking NodePort Assignments")
		rc, err := ing.t.KubeClient.Core().ReplicationControllers(TestNamespace).Get(testServerRc.Name)
		if err == nil {

			svc, err := ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
			if err != nil {
				return errors.New().WithMessage("Service get encountered error").Err()
			}
			// Removing pods so that endpoints get updated
			rc.Spec.Replicas = 0
			ing.t.KubeClient.Core().ReplicationControllers(TestNamespace).Update(rc)

			for {
				pods, _ := ing.t.KubeClient.Core().Pods(TestNamespace).List(api.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set(rc.Spec.Selector)),
				})
				if len(pods.Items) <= 0 {
					break
				}
				time.Sleep(time.Second * 5)
			}
			svcUpdated, err := ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
			if err != nil {
				return errors.New().WithMessage("Service get encountered error").Err()
			}

			for _, oldPort := range svc.Spec.Ports {
				for _, newPort := range svcUpdated.Spec.Ports {
					if oldPort.Port == newPort.Port {
						if oldPort.NodePort != newPort.NodePort {
							return errors.New().WithMessage("Node Port Mismatched").Err()
						}
					}
				}
			}

			rc.Spec.Replicas = 2
			ing.t.KubeClient.Core().ReplicationControllers(TestNamespace).Update(rc)

			svcUpdated, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
			if err != nil {
				return errors.New().WithMessage("Service get encountered error").Err()
			}

			for _, oldPort := range svc.Spec.Ports {
				for _, newPort := range svcUpdated.Spec.Ports {
					if oldPort.Port == newPort.Port {
						if oldPort.NodePort != newPort.NodePort {
							return errors.New().WithMessage("Node Port Mismatched").Err()
						}
					}
				}
			}
		}
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressDaemonUpdate() error {
	if !ing.t.Config.InCluster && ing.t.Config.ProviderName != "minikube" {
		log.Infoln("Test is Running from outside of cluster skipping test")
		return nil
	}

	var nodeSelector = func() string {
		if ing.t.Config.ProviderName == "minikube" {
			return "kubernetes.io/hostname=minikube"
		} else {
			if len(ing.t.Config.DaemonHostName) > 0 {
				return "kubernetes.io/hostname=" + ing.t.Config.DaemonHostName
			}
			return "kubernetes.io/hostname=" + ing.t.Config.ClusterName + "-master"
		}
		return ""
	}
	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
			Annotations: map[string]string{
				ingress.LBType:             ingress.LBHostPort,
				ingress.DaemonNodeSelector: nodeSelector(),
			},
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	for i := 0; i < maxRetries; i++ {
		_, err := ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}

	serverAddr, err := ing.getDaemonURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 20)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}

	updatedBaseIngress, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Get(baseIngress.Name)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}
	updatedBaseIngress.Spec.Rules[0].HTTP.Paths[0].Path = "/newTestpath"
	_, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Update(updatedBaseIngress)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}

	time.Sleep(time.Second * 30)
	serverAddr, err = ing.getDaemonURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 30)
	log.Infoln("Loadbalancer created, calling http endpoints for updated path, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(1)
		if err == nil {
			return errors.New().WithCause(err).WithMessage("Connected with old prefix").Err()
		}
		log.Infoln("Expected exception, faild to connect with old path, calling new paths.")
		resp, err = testserverclient.NewTestHTTPClient(url).Method("GET").Path("/newTestpath/ok").DoWithRetry(5)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to Connect With New Prefix").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/newTestpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}

	// Open New Port
	updatedBaseIngress, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Get(baseIngress.Name)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}

	if ing.t.Config.ProviderName != "minikube" {
		updatedBaseIngress.Spec.Rules[0].HTTP = nil
		updatedBaseIngress.Spec.Rules[0].TCP = []aci.TCPExtendedIngressRuleValue{
			{
				Port: intstr.FromString("4545"),
				Backend: aci.IngressBackend{
					ServiceName: testServerSvc.Name,
					ServicePort: intstr.FromString("4545"),
				},
			},
		}
		_, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Update(updatedBaseIngress)
		if err != nil {
			return errors.New().WithCause(err).Err()
		}
		time.Sleep(time.Second * 30)

		found := false
		for i := 1; i <= maxRetries; i++ {
			svc, err := ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
			if err != nil {
				continue
			}
			log.Infoln("Got Service", svc.Name)
			for _, port := range svc.Spec.Ports {
				log.Infoln(port)
				if port.Port == 4545 {
					found = true
					break
				}
			}
			if found {
				break
			}
			time.Sleep(time.Second * 5)
		}

		if !found {
			return errors.New().WithMessage("Service not found").Err()
		}

		serverAddr, err = ing.getDaemonURLs(baseIngress)
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 30)
		log.Infoln("Loadbalancer created, calling http endpoints for updated path, Total", len(serverAddr))
		for _, url := range serverAddr {
			resp, err := testserverclient.NewTestTCPClient(url).DoWithRetry(50)
			if err != nil {
				return errors.New().WithCause(err).WithMessage("Failed to Connect With New Prefix").Err()
			}
			log.Infoln("Response", *resp)
			if resp.ServerPort != ":4545" {
				return errors.New().WithMessage("Port did not matched").Err()
			}
		}
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressCreateIPPersist() error {
	if len(ing.t.Config.LBPersistIP) > 0 &&
		(ing.t.Config.ProviderName == "gce" ||
			ing.t.Config.ProviderName == "gke" ||
			(ing.t.Config.ProviderName == "aws" && ing.t.Config.InCluster)) {
		baseIngress := &aci.Ingress{
			ObjectMeta: api.ObjectMeta{
				Name:      testIngressName(),
				Namespace: TestNamespace,
				Annotations: map[string]string{
					ingress.LoadBalancerPersist: "true",
					ingress.LoadBalancerIP:      ing.t.Config.LBPersistIP,
				},
			},
			Spec: aci.ExtendedIngressSpec{
				Rules: []aci.ExtendedIngressRule{
					{
						ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
							HTTP: &aci.HTTPExtendedIngressRuleValue{
								Paths: []aci.HTTPExtendedIngressPath{
									{
										Path: "/testpath",
										Backend: aci.ExtendedIngressBackend{
											ServiceName: testServerSvc.Name,
											ServicePort: intstr.FromInt(80),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		_, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
		if err != nil {
			return err
		}
		defer func() {
			if ing.t.Config.Cleanup {
				ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
			}
		}()

		// Wait sometime to loadbalancer be opened up.
		time.Sleep(time.Second * 10)
		var svc *api.Service
		for i := 0; i < maxRetries; i++ {
			svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
			if err == nil {
				break
			}
			time.Sleep(time.Second * 5)
			log.Infoln("Waiting for service to be created")
		}
		if err != nil {
			return err
		}
		log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
		for i := 0; i < maxRetries; i++ {
			_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
			if err == nil {
				break
			}
			time.Sleep(time.Second * 5)
			log.Infoln("Waiting for endpoints to be created")
		}
		if err != nil {
			return err
		}

		serverAddr, err := ing.getURLs(baseIngress)
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 30)
		log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
		for _, url := range serverAddr {
			resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
			if err != nil {
				return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
			}
			log.Infoln("Response", *resp)
			if resp.Method != http.MethodGet {
				return errors.New().WithMessage("Method did not matched").Err()
			}

			if resp.Path != "/testpath/ok" {
				return errors.New().WithMessage("Path did not matched").Err()
			}
		}

		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err != nil {
			return errors.New().WithCause(err).Err()
		}
		oldServiceIP := svc.Status.LoadBalancer.Ingress[0].IP

		err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		if err != nil {
			return errors.New().WithCause(err).Err()
		}

		time.Sleep(time.Second * 30)
		baseIngress = &aci.Ingress{
			ObjectMeta: api.ObjectMeta{
				Name:      testIngressName(),
				Namespace: TestNamespace,
				Annotations: map[string]string{
					ingress.LoadBalancerPersist: "true",
					ingress.LoadBalancerIP:      oldServiceIP,
				},
			},
			Spec: aci.ExtendedIngressSpec{
				Rules: []aci.ExtendedIngressRule{
					{
						ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
							HTTP: &aci.HTTPExtendedIngressRuleValue{
								Paths: []aci.HTTPExtendedIngressPath{
									{
										Path: "/testpath",
										Backend: aci.ExtendedIngressBackend{
											ServiceName: testServerSvc.Name,
											ServicePort: intstr.FromInt(80),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		_, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
		if err != nil {
			return err
		}
		defer func() {
			if ing.t.Config.Cleanup {
				ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
			}
		}()

		// Wait sometime to loadbalancer be opened up.
		time.Sleep(time.Second * 10)
		for i := 0; i < maxRetries; i++ {
			svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
			if err == nil {
				break
			}
			time.Sleep(time.Second * 5)
			log.Infoln("Waiting for service to be created")
		}
		if err != nil {
			return err
		}
		log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
		for i := 0; i < maxRetries; i++ {
			_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
			if err == nil {
				break
			}
			time.Sleep(time.Second * 5)
			log.Infoln("Waiting for endpoints to be created")
		}
		if err != nil {
			return err
		}

		serverAddr, err = ing.getURLs(baseIngress)
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 30)
		log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
		for _, url := range serverAddr {
			resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
			if err != nil {
				return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
			}
			log.Infoln("Response", *resp)
			if resp.Method != http.MethodGet {
				return errors.New().WithMessage("Method did not matched").Err()
			}

			if resp.Path != "/testpath/ok" {
				return errors.New().WithMessage("Path did not matched").Err()
			}
		}

		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err != nil {
			return errors.New().WithCause(err).Err()
		}

		found := false
		for _, lbIngress := range svc.Status.LoadBalancer.Ingress {
			log.Infoln("Matching Service Ips for Ingress", lbIngress.IP, oldServiceIP)
			if lbIngress.IP == oldServiceIP {
				found = true
				break
			}
		}

		if !found {
			log.Infoln("Service Ip not matched with previous IP")
			return errors.New().WithMessage("Service Ip not matched with previous IP").Err()
		}
	} else {
		log.Infoln("Test Skipped")
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressCreateWithOptions() error {
	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
										HeaderRule: []string{
											"X-Ingress-Test-Header ingress.appscode.com",
										},
										RewriteRule: []string{
											`^([^\ :]*)\ /(.*)$ \1\ /override/\2`,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	var svc *api.Service
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}
	log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
	for i := 0; i < maxRetries; i++ {
		_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for endpoints to be created")
	}
	if err != nil {
		return err
	}

	serverAddr, err := ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 30)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/override/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}

		if resp.RequestHeaders.Get("X-Ingress-Test-Header") != "ingress.appscode.com" {
			return errors.New().WithMessage("Header did not matched").Err()
		}
	}

	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).
			Method("GET").
			Header(map[string]string{
				"X-Ingress-Test-Header": "ingress.appscode.com/v1beta1",
			}).
			Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/override/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}

		if resp.RequestHeaders.Get("X-Ingress-Test-Header") != "ingress.appscode.com/v1beta1" {
			return errors.New().WithMessage("Header did not matched").Err()
		}
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressCoreIngress() error {
	baseIngress := &extensions.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "voyager",
			},
		},
		Spec: extensions.IngressSpec{
			Rules: []extensions.IngressRule{
				{
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path: "/testpath",
									Backend: extensions.IngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.KubeClient.Extensions().Ingresses(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.KubeClient.Extensions().Ingresses(baseIngress.Namespace).Delete(baseIngress.Name, &api.DeleteOptions{})
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	var svc *api.Service
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}
	log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
	for i := 0; i < maxRetries; i++ {
		_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for endpoints to be created")
	}
	if err != nil {
		return err
	}

	baseExtIngress, err := aci.NewEngressFromIngress(baseIngress)
	if err != nil {
		return errors.New().WithCause(err).Err()
	}
	serverAddr, err := ing.getURLs(baseExtIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 30)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressHostNames() error {
	headlessSvc, err := ing.t.KubeClient.Core().Services(TestNamespace).Create(testStatefulSetSvc)
	if err != nil {
		return err
	}
	orphan := false
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.KubeClient.Core().Services(TestNamespace).Delete(headlessSvc.Name, &api.DeleteOptions{
				OrphanDependents: &orphan,
			})
		}
	}()

	ss, err := ing.t.KubeClient.Apps().StatefulSets(TestNamespace).Create(testServerStatefulSet)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.KubeClient.Apps().StatefulSets(TestNamespace).Delete(ss.Name, &api.DeleteOptions{
				OrphanDependents: &orphan,
			})
		}
	}()

	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										HostNames:   []string{testServerStatefulSet.Name + "-0"},
										ServiceName: headlessSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	var svc *api.Service
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}
	log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
	for i := 0; i < maxRetries; i++ {
		_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for endpoints to be created")
	}
	if err != nil {
		return err
	}

	serverAddr, err := ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 30)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}
		if resp.PodName != ss.Name+"-0" {
			return errors.New().WithMessage("PodName did not matched").Err()
		}
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressDaemonRestart() error {
	if !ing.t.Config.InCluster && ing.t.Config.ProviderName != "minikube" {
		log.Infoln("Test is Running from outside of cluster skipping test")
		return nil
	}

	var nodeSelector = func() string {
		if ing.t.Config.ProviderName == "minikube" {
			return "kubernetes.io/hostname=minikube"
		} else {
			if len(ing.t.Config.DaemonHostName) > 0 {
				return "kubernetes.io/hostname=" + ing.t.Config.DaemonHostName
			}
			return "kubernetes.io/hostname=" + ing.t.Config.ClusterName + "-master"
		}
		return ""
	}

	baseDaemonIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
			Annotations: map[string]string{
				ingress.LBType:             ingress.LBHostPort,
				ingress.DaemonNodeSelector: nodeSelector(),
			},
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseDaemonIngress.Namespace).Create(baseDaemonIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseDaemonIngress.Namespace).Delete(baseDaemonIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	for i := 0; i < maxRetries; i++ {
		_, err := ing.t.KubeClient.Core().Services(baseDaemonIngress.Namespace).Get(ingress.VoyagerPrefix + baseDaemonIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return errors.New().WithCause(err).Err()
	}

	serverAddr, err := ing.getDaemonURLs(baseDaemonIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 20)
	log.Infoln("Loadbalancer created, calling http endpoints for test, Total url found", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}

	// Teardown and then again create one pod of the backend
	// And Make sure The DS does not gets deleted.
	_, err = ing.t.KubeClient.Extensions().DaemonSets(baseDaemonIngress.Namespace).Get(ingress.VoyagerPrefix + baseDaemonIngress.Name)
	if err != nil {
		return err
	}
	rc, err := ing.t.KubeClient.Core().ReplicationControllers(testServerRc.Namespace).Get(testServerRc.Name)
	if err != nil {
		return err
	}
	rc.Spec.Replicas -= 1
	rc, err = ing.t.KubeClient.Core().ReplicationControllers(testServerRc.Namespace).Update(rc)
	if err != nil {
		return err
	}
	_, err = ing.t.KubeClient.Extensions().DaemonSets(baseDaemonIngress.Namespace).Get(ingress.VoyagerPrefix + baseDaemonIngress.Name)
	if err != nil {
		return err
	}
	rc, err = ing.t.KubeClient.Core().ReplicationControllers(testServerRc.Namespace).Get(testServerRc.Name)
	if err != nil {
		return err
	}
	rc.Spec.Replicas += 1
	rc, err = ing.t.KubeClient.Core().ReplicationControllers(testServerRc.Namespace).Update(rc)
	if err != nil {
		return err
	}
	_, err = ing.t.KubeClient.Extensions().DaemonSets(baseDaemonIngress.Namespace).Get(ingress.VoyagerPrefix + baseDaemonIngress.Name)
	if err != nil {
		return err
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressBackendWeight() error {
	dp1, err := ing.t.KubeClient.Extensions().Deployments(TestNamespace).Create(&extensions.Deployment{
		ObjectMeta: api.ObjectMeta{
			Name:      "deploymet-1-" + randString(4),
			Namespace: TestNamespace,
		},
		Spec: extensions.DeploymentSpec{
			Replicas: 1,
			Selector: &unversioned.LabelSelector{
				MatchLabels: map[string]string{
					"app":         "deployment",
					"app-version": "v1",
				},
			},
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{
						"app":         "deployment",
						"app-version": "v1",
					},
					Annotations: map[string]string{
						ingress.LoadBalancerBackendWeight: "90",
					},
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Name:  "server",
							Image: "appscode/test-server:1.1",
							Env: []api.EnvVar{
								{
									Name: "POD_NAME",
									ValueFrom: &api.EnvVarSource{
										FieldRef: &api.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							Ports: []api.ContainerPort{
								{
									Name:          "http-1",
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	dp2, err := ing.t.KubeClient.Extensions().Deployments(TestNamespace).Create(&extensions.Deployment{
		ObjectMeta: api.ObjectMeta{
			Name:      "deploymet-2-" + randString(4),
			Namespace: TestNamespace,
		},
		Spec: extensions.DeploymentSpec{
			Replicas: 1,
			Selector: &unversioned.LabelSelector{
				MatchLabels: map[string]string{
					"app":         "deployment",
					"app-version": "v2",
				},
			},
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{
						"app":         "deployment",
						"app-version": "v2",
					},
					Annotations: map[string]string{
						ingress.LoadBalancerBackendWeight: "10",
					},
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Name:  "server",
							Image: "appscode/test-server:1.1",
							Env: []api.EnvVar{
								{
									Name: "POD_NAME",
									ValueFrom: &api.EnvVarSource{
										FieldRef: &api.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							Ports: []api.ContainerPort{
								{
									Name:          "http-1",
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	svc, err := ing.t.KubeClient.Core().Services(TestNamespace).Create(&api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:      "deployment-svc",
			Namespace: TestNamespace,
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{
				{
					Name:       "http-1",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   "TCP",
				},
			},
			Selector: map[string]string{
				"app": "deployment",
			},
		},
	})
	if err != nil {
		return err
	}

	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/testpath",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: svc.Name,
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	defer func() {
		if ing.t.Config.Cleanup {
			orphan := false
			ing.t.KubeClient.Extensions().Deployments(dp1.Namespace).Delete(dp1.Name, &api.DeleteOptions{
				OrphanDependents: &orphan,
			})

			ing.t.KubeClient.Extensions().Deployments(dp2.Namespace).Delete(dp2.Name, &api.DeleteOptions{
				OrphanDependents: &orphan,
			})

			ing.t.KubeClient.Core().Services(svc.Namespace).Delete(svc.Name, &api.DeleteOptions{
				OrphanDependents: &orphan,
			})
		}
	}()

	_, err = ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	time.Sleep(time.Second * 10)
	for i := 0; i < maxRetries; i++ {
		_, err := ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return errors.New().WithCause(err).Err()
	}

	serverAddr, err := ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 20)
	log.Infoln("Loadbalancer created, calling http endpoints for test, Total url found", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/testpath/ok" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}
	var deploymentCounter1, deploymentCounter2 int
	for cnt := 0; cnt < 100; cnt++ {
		for _, url := range serverAddr {
			resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/testpath/ok").DoWithRetry(50)
			if err != nil {
				return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
			}
			log.Infoln("Response", *resp)
			if resp.Method != http.MethodGet {
				return errors.New().WithMessage("Method did not matched").Err()
			}

			if strings.HasPrefix(resp.PodName, dp1.Name) {
				deploymentCounter1++
			} else if strings.HasPrefix(resp.PodName, dp2.Name) {
				deploymentCounter2++
			}
		}
	}

	if deploymentCounter2 != 10 {
		return errors.New().WithMessagef("Expected %v Actual %v", 10, deploymentCounter2).Err()
	}

	if deploymentCounter1 != 90 {
		return errors.New().WithMessagef("Expected %v Actual %v", 90, deploymentCounter1).Err()
	}
	return nil
}

func (ing *IngressTestSuit) TestIngressBackendRule() error {
	baseIngress := &aci.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      testIngressName(),
			Namespace: TestNamespace,
		},
		Spec: aci.ExtendedIngressSpec{
			Rules: []aci.ExtendedIngressRule{
				{
					ExtendedIngressRuleValue: aci.ExtendedIngressRuleValue{
						HTTP: &aci.HTTPExtendedIngressRuleValue{
							Paths: []aci.HTTPExtendedIngressPath{
								{
									Path: "/old",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
										BackendRule: []string{
											"acl add_url capture.req.uri -m beg /old/add/now",
											`http-response set-header X-Added-From-Proxy added-from-proxy if add_url`,

											"acl rep_url path_beg /old/replace",
											`reqrep ^([^\ :]*)\ /(.*)$ \1\ /rewrited/from/proxy/\2 if rep_url`,
										},
									},
								},
								{
									Path: "/test-second",
									Backend: aci.ExtendedIngressBackend{
										ServiceName: testServerSvc.Name,
										ServicePort: intstr.FromInt(80),
										BackendRule: []string{
											"acl add_url capture.req.uri -m beg /test-second",
											`http-response set-header X-Added-From-Proxy added-from-proxy if add_url`,

											"acl rep_url path_beg /test-second",
											`reqrep ^([^\ :]*)\ /(.*)$ \1\ /rewrited/from/proxy/\2 if rep_url`,
										},
										HeaderRule: []string{
											"X-Ingress-Test-Header ingress.appscode.com",
										},
										RewriteRule: []string{
											`^([^\ :]*)\ /(.*)$ \1\ /override/\2`,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Create(baseIngress)
	if err != nil {
		return err
	}
	defer func() {
		if ing.t.Config.Cleanup {
			ing.t.ExtensionClient.Ingress(baseIngress.Namespace).Delete(baseIngress.Name)
		}
	}()

	// Wait sometime to loadbalancer be opened up.
	time.Sleep(time.Second * 10)
	var svc *api.Service
	for i := 0; i < maxRetries; i++ {
		svc, err = ing.t.KubeClient.Core().Services(baseIngress.Namespace).Get(ingress.VoyagerPrefix + baseIngress.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for service to be created")
	}
	if err != nil {
		return err
	}
	log.Infoln("Service Created for loadbalancer, Checking for service endpoints")
	for i := 0; i < maxRetries; i++ {
		_, err = ing.t.KubeClient.Core().Endpoints(svc.Namespace).Get(svc.Name)
		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
		log.Infoln("Waiting for endpoints to be created")
	}
	if err != nil {
		return err
	}

	serverAddr, err := ing.getURLs(baseIngress)
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 30)
	log.Infoln("Loadbalancer created, calling http endpoints, Total", len(serverAddr))
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/old/replace").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/rewrited/from/proxy/old/replace" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}
	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/old/add/now").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Method did not matched").Err()
		}

		if resp.Path != "/old/add/now" {
			return errors.New().WithMessage("Path did not matched").Err()
		}

		if resp.ResponseHeader.Get("X-Added-From-Proxy") != "added-from-proxy" {
			return errors.New().WithMessage("Header did not matched").Err()
		}
	}

	for _, url := range serverAddr {
		resp, err := testserverclient.NewTestHTTPClient(url).Method("GET").Path("/test-second").DoWithRetry(50)
		if err != nil {
			return errors.New().WithCause(err).WithMessage("Failed to connect with server").Err()
		}
		log.Infoln("Response", *resp)
		if resp.Method != http.MethodGet {
			return errors.New().WithMessage("Metho/replaced did not matched").Err()
		}

		if resp.RequestHeaders.Get("X-Ingress-Test-Header") != "ingress.appscode.com" {
			return errors.New().WithMessage("Header did not matched").Err()
		}

		if resp.ResponseHeader.Get("X-Added-From-Proxy") != "added-from-proxy" {
			return errors.New().WithMessage("Header did not matched").Err()
		}

		if resp.RequestHeaders.Get("X-Ingress-Test-Header") != "ingress.appscode.com" {
			return errors.New().WithMessage("Header did not matched").Err()
		}

		if resp.Path != "/override/rewrited/from/proxy/test-second" {
			return errors.New().WithMessage("Path did not matched").Err()
		}
	}
	return nil
}
