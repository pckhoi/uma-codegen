package uma

import (
	"fmt"
	"net/http"
)

type ProviderInfoGetter func(r *http.Request) ProviderInfo

type middleware struct {
	rm              *resourceMatcher
	getProviderInfo ProviderInfoGetter
	providers       map[string]Provider
	resourceStore   ResourceStore
	client          *http.Client
}

func (m *middleware) getProvider(r *http.Request) (*http.Request, Provider) {
	pinfo := m.getProviderInfo(r)
	if v, ok := m.providers[pinfo.Issuer]; ok {
		return setProvider(r, v), v
	}
	var p Provider
	switch pinfo.Type {
	case Keycloak:
		p = &keycloakProvider{
			baseProvider: newBaseProvider(pinfo.Issuer, pinfo.ClientID, pinfo.ClientSecret, pinfo.KeySet, m.resourceStore, m.client),
		}
	default:
		panic(fmt.Errorf("unsupported provider type %q", pinfo.Type))
	}
	m.providers[pinfo.Issuer] = p
	if err := p.DiscoverUMA(); err != nil {
		panic(err)
	}
	return setProvider(r, p), p
}

type ResourceMiddlewareOptions struct {
	// ====== required fields ======

	// GetBaseURL returns the base url of the covered api. It is typically the "url" of the matching
	// server entry in openapi spec. It should have this format: "{SCHEME}://{PUBLIC_HOSTNAME}{ANY_BASE_PATH}"
	GetBaseURL URLGetter

	// GetProviderInfo returns the provider info given the request. It allows you to use different UMA
	// providers for different requests if you so wish
	GetProviderInfo ProviderInfoGetter

	// ResourceStore persistently stores resource name and id. Some UMA providers don't like to be told
	// twice about the same resource. This tells the middleware which resource is already registered so
	// it doesn't have to be registered again.
	ResourceStore ResourceStore

	// Types maps resource type name to their description. Typically generated by uma-codegen
	Types map[string]ResourceType

	// ResourceTemplates are the template objects to generate resource object on the fly. Typically generated
	// by uma-codegen
	ResourceTemplates ResourceTemplates

	// ====== optional fields ======

	// Client is the http client that the middlewares can use to communicate with UMA providers. Defaults to
	// http.DefaultClient
	Client *http.Client
}

// ResourceMiddleware detects UMAResource by matching request path with paths. types is the map between
// resource type and UMAResourceType. paths is the map between path template and resouce type as defined
// in OpenAPI spec.
func ResourceMiddleware(opts ResourceMiddlewareOptions) func(next http.Handler) http.Handler {
	rm := newResourceMatcher(opts.GetBaseURL, opts.Types, opts.ResourceTemplates)
	m := &middleware{
		rm:              rm,
		getProviderInfo: opts.GetProviderInfo,
		providers:       map[string]Provider{},
		resourceStore:   opts.ResourceStore,
		client:          http.DefaultClient,
	}
	if opts.Client != nil {
		m.client = opts.Client
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var resource *Resource
			r, resource = m.rm.match(r)
			if resource != nil {
				var p Provider
				r, p = m.getProvider(r)
				if err := p.RegisterResource(resource); err != nil {
					panic(err)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
