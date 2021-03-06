package ingress

import (
	"strconv"
	"strings"
	"sync"

	aci "github.com/appscode/k8s-addons/api"
	acs "github.com/appscode/k8s-addons/client/clientset"
	"github.com/appscode/k8s-addons/pkg/stash"
	"k8s.io/kubernetes/pkg/client/cache"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

const (
	VoyagerPrefix = "voyager-"

	stickySession = "ingress.appscode.com/stickySession"

	// LB stats options
	StatPort    = 1936
	StatsOn     = "ingress.appscode.com/stats"
	StatsSecret = "ingress.appscode.com/stats.secretName"

	// Daemon, Persistent, LoadBalancer
	LBName     = "ingress.appscode.com/name"
	LBType     = "ingress.appscode.com/type"
	LBNodePort = "NodePort"
	LBHostPort = "HostPort"
	// Deprecated, use LBHostPort
	LBDaemon       = "Daemon"
	LBLoadBalancer = "LoadBalancer" // default

	// Runs on a specific set of a hosts via DaemonSet. This is needed to work around the issue that master node is registered but not scheduable.
	DaemonNodeSelector = "ingress.appscode.com/daemon.nodeSelector"

	// Replicas specify # of HAProxy pods run (default 1)
	Replicas = "ingress.appscode.com/replicas"

	// LoadBalancer mode exposes HAProxy via a type=LoadBalancer service. This is the original version implemented by @sadlil
	// Uses nodeport and Cloud LoadBalancer exists beyond single HAProxy run
	LoadBalancerIP      = "ingress.appscode.com/ip"      // external_ip or loadbalancer_ip "" or a "ipv4"
	LoadBalancerPersist = "ingress.appscode.com/persist" // "" or a "true"

	// LoadBalancerBackendWeightKey is the weight value of a Pod that was
	// addressed by the Endpoint, this weight will be added to server backend.
	// Traffic will be forwarded according to there weight.
	LoadBalancerBackendWeight = "ingress.appscode.com/backend.weight"
)

type annotation map[string]string

func (s annotation) StickySession() bool {
	_, ok := s[stickySession]
	return ok
}

func (s annotation) Stats() bool {
	_, ok := s[StatsOn]
	return ok
}

func (s annotation) StatsSecretName() string {
	v, _ := s[StatsSecret]
	return v
}

func (s annotation) LBType() string {
	if v, ok := s[LBType]; ok {
		return v
	}
	return LBLoadBalancer
}

func (s annotation) Replicas() int32 {
	if v, ok := s[Replicas]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return int32(n)
		}
		return 1
	}
	return 1
}

func (s annotation) DaemonNodeSelector() string {
	v, _ := s[DaemonNodeSelector]
	return v
}

func (s annotation) LoadBalancerIP() string {
	v, _ := s[LoadBalancerIP]
	return v
}

func (s annotation) LoadBalancerPersist() bool {
	v, _ := s[LoadBalancerPersist]
	return strings.ToLower(v) == "true"
}

type EngressController struct {
	// kubernetes client
	KubeClient        clientset.Interface
	ACExtensionClient acs.AppsCodeExtensionInterface
	CloudManager      cloudprovider.Interface

	// Engress object that created or updated.
	Config  *aci.Ingress
	Options *KubeOptions
	// contains all the https host names.
	HostFilter []string

	// parsed ingress.
	Parsed *HAProxyOptions

	// endpoint cache store. contains all endpoints will be
	// search with respect to services.
	Storage       *stash.Storage
	EndpointStore cache.StoreToEndpointsLister

	LoadbalancerImage string

	sync.Mutex

	IngressClass string
}

type KubeOptions struct {
	// name of the cluster the daemon running.
	ClusterName string

	ProviderName string
	// kube options data
	SecretNames []string

	ConfigMapName string
	// contains raw configMap data parsed from the cfg file.
	ConfigData string

	// Ports contains all the ports needed to be opened for the ingress.
	// Those ports will be used to open loadbalancer/firewall.
	// So any interference with underlying endpoints will not cause network update.
	Ports []int

	LBType              string
	Replicas            int32
	DaemonNodeSelector  map[string]string
	LoadBalancerIP      string
	LoadBalancerPersist bool
}

func (o KubeOptions) SupportsLoadBalancerType() bool {
	return o.ProviderName == "aws" ||
		o.ProviderName == "gce" ||
		o.ProviderName == "gke" ||
		o.ProviderName == "azure" ||
		o.ProviderName == "minikube"
}

type HAProxyOptions struct {
	Timestamp int64
	// those options are get from annotations. applied globally
	// in all the sections.

	// stick requests to specified servers.
	Sticky  bool
	SSLCert bool

	// open up load balancer stats
	Stats bool
	// Basic auth to lb stats
	StatsUserName string
	StatsPassWord string

	DefaultBackend *Backend
	HttpsService   []*Service
	HttpService    []*Service
	TCPService     []*TCPService
}

type Service struct {
	Name     string
	AclMatch string
	Host     string
	Backends *Backend
}

type TCPService struct {
	Name        string
	Host        string
	Port        string
	SecretName  string
	PEMName     string
	Backends    *Backend
	ALPNOptions string
}

type Backend struct {
	Name         string   `json:"Name,omitempty"`
	BackendRules []string `json:"BackendRules,omitempty"`
	// Deprecated
	RewriteRules []string `json:"RewriteRules,omitempty"`
	// Deprecated
	HeaderRules []string    `json:"HeaderRules,omitempty"`
	Endpoints   []*Endpoint `json:"Endpoints,omitempty"`
}

type Endpoint struct {
	Name   string
	IP     string
	Port   string
	Weight int
}

// Loadbalancer image is an almost constant type.
// this will only be set at the runtime but only for once.
// once this is set the value can not be changed.
var loadbalancerImage string

func SetLoadbalancerImage(i string) {
	var once sync.Once
	once.Do(
		func() {
			loadbalancerImage = i
		},
	)
}

func GetLoadbalancerImage() string {
	return loadbalancerImage
}
