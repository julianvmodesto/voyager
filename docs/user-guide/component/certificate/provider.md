# Configuring your challenge provider(s)

## DNS Providers
Voyager uses kubernetes secret within the pod to fetch credentials required for various DNS providers.
Making those correctly accessible to Voyager will require specifying the secret name inside an certificate objects.
The Secret will need the Key name exactly provided.

### HTTP (beta)
HTTP Provider will requires an [Ingress](/docs/user-guide/component/ingress) reference to resolve with.
Reference an Ingress name for http provider. Ingress IP should be setted as domain A record in its provider.
Read how to create certificate using [HTTP Provider](/docs/user-guide/certificate/create.md#create-certificate-with-http-provider)

### Cloudflare
`CLOUDFLARE_EMAIL`: The email of the cloudflare user <br>
`CLOUDFLARE_API_KEY`: The API key corresponding to the email <br>

### Digital Ocean
`DO_AUTH_TOKEN`: The digital ocean authorization token <br>

### DNSimple
`DNSIMPLE_EMAIL`: The email fo the DNSimple user <br>
`DNSIMPLE_API_KEY`: The API key corresponding to the email <br>

### DNS Made Easy
`DNSMADEEASY_API_KEY`: The API key for DNS Made Easy <br>
`DNSMADEEASY_API_SECRET`: The api secret corresponding with the API key <br>
`DNSMADEEASY_SANDBOX`: A boolean flag, if set to true or 1, requests will be sent to the sandbox API <br>

### Dyn
`DYN_CUSTOMER_NAME`: The customer name of the Dyn user <br>
`DYN_USER_NAME`: The user name of the Dyn user <br>
`DYN_PASSWORD`: The password of the Dyn user <br>

### Gandi
`GANDI_API_KEY`: The API key for Gandi <br>

### Google Cloud
`GCE_PROJECT`: The name of the Google Cloud project to use <br>
`GOOGLE_APPLICATION_CREDENTIALS`: Credential Data. <br>

### Namecheap
`NAMECHEAP_API_USER`: The username of the namecheap user <br>
`NAMECHEAP_API_KEY`: The API key corresponding with the namecheap user <br>

### OVH
`OVH_ENDPOINT`: The URL of the API endpoint to use <br>
`OVH_APPLICATION_KEY`: The application key <br>
`OVH_APPLICATION_SECRET`: The secret corresponding to the application key <br>
`OVH_CONSUMER_KEY`: The consumer key <br>

### PDNS
`PDNS_API_KEY`: The API key to use <br>

### RFC2136
The rfc2136 provider works with any DNS provider implementing the DNS Update rfc2136.
the TSIG variables need only be set if using TSIG authentication.

`RFC2136_NAMESERVER`: The network address of the provider, in the form of "host" or "host:port" <br>
`RFC2136_TSIG_ALGORITHM`: The algorithm to use for TSIG authentication. <br>
`RFC2136_TSIG_KEY`: The key to use for TSIG authentication. <br>
`RFC2136_TSIG_SECRET`: The secret to use for TSIG authentication. <br>

### Amazon Route53
`AWS_ACCESS_KEY_ID`: The access key ID <br>
`AWS_SECRET_ACCESS_KEY`: The secret corresponding to the access key <br>

### Vultr
`VULTR_API_KEY`: The API key to use <br>

### Linode
`LINODE_API_KEY`: API Key for linode to use. <br>

An Example Secret would look like
```yaml
kind: Secret
metadata:
  name: ssl-appscode-io
  namespace: default
data:
  GCE_PROJECT: <project-name>
  GOOGLE_APPLICATION_CREDENTIALS: <credential>
```