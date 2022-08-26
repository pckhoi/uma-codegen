package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/dnaeon/go-vcr/v2/recorder"
	"github.com/pckhoi/uma"
)

type mockResourceStore map[string]string

func (s mockResourceStore) Set(name, id string) {
	s[name] = id
}

func (s mockResourceStore) Get(name string) string {
	id, ok := s[name]
	if !ok {
		return ""
	}
	return id
}

func createKeycloakProvider(vcrDir string) (*uma.KeycloakProvider, func() error) {
	r, err := recorder.NewAsMode(filepath.Join(vcrDir, "test_server"), recorder.ModeReplaying, http.DefaultTransport)
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{}
	*client = *http.DefaultClient
	client.Transport = r
	issuer := "http://localhost:8080/realms/test-realm"
	kp, err := uma.NewKeycloakProvider(
		issuer, "test-client", "change-me",
		oidc.NewRemoteKeySet(oidc.ClientContext(context.Background(), client), issuer+"/protocol/openid-connect/certs"),
		client, true,
	)
	if err != nil {
		log.Fatal(err)
	}
	return kp, r.Stop
}

func wrappInMiddleware(kp uma.Provider, rs uma.ResourceStore, port string, handler http.Handler) http.Handler {
	return UMAMiddleware(uma.MiddlewareOptions{
		GetBaseURL: func(r *http.Request) url.URL {
			return url.URL{
				Scheme: "http",
				Host:   "localhost:" + port,
				Path:   "/users",
			}
		},
		GetProvider: func(r *http.Request) uma.Provider {
			return kp
		},
		ResourceStore:                  rs,
		DisableTokenExpirationCheck:    true,
		IncludeScopeInPermissionTicket: true,
	})(handler)
}

func main() {
	port := os.Args[1]
	vcrDir := os.Args[2]
	rs := make(mockResourceStore)
	kp, stop := createKeycloakProvider(vcrDir)
	defer stop()
	sm := http.NewServeMux()
	sm.HandleFunc("/register-resources", func(w http.ResponseWriter, r *http.Request) {
		resp, err := kp.CreateResource(&uma.Resource{
			ResourceType: uma.ResourceType{
				Type:           "https://www.example.com/rsrcs/users",
				IconUri:        "https://www.example.com/rsrcs/users/icon.png",
				ResourceScopes: []string{"read", "write"},
			},
			Name: "Users",
			URI:  fmt.Sprintf("http://localhost:%s/users", port),
		})
		if err != nil {
			panic(err)
		}
		rs.Set(resp.Name, resp.ID)
		_, err = kp.CreatePermissionForResource(resp.ID, &uma.KcPermission{
			Name:        "reader-read-" + resp.ID,
			Description: "Reader can read users",
			Scopes:      []string{"read"},
			Roles:       []string{"reader"},
		})
		if err != nil {
			panic(err)
		}
		_, err = kp.CreatePermissionForResource(resp.ID, &uma.KcPermission{
			Name:        "writer-write-" + resp.ID,
			Description: "Writer can write users",
			Scopes:      []string{"write"},
			Roles:       []string{"writer"},
		})
		if err != nil {
			panic(err)
		}
		resp, err = kp.CreateResource(&uma.Resource{
			ResourceType: uma.ResourceType{
				Type:           "https://www.example.com/rsrcs/user",
				IconUri:        "https://www.example.com/rsrcs/user/icon.png",
				ResourceScopes: []string{"read", "write"},
			},
			Name: "User 1",
			URI:  fmt.Sprintf("http://localhost:%s/users/1", port),
		})
		if err != nil {
			panic(err)
		}
		rs.Set(resp.Name, resp.ID)
		_, err = kp.CreatePermissionForResource(resp.ID, &uma.KcPermission{
			Name:        "reader-read-" + resp.ID,
			Description: "Reader can read user",
			Scopes:      []string{"read"},
			Roles:       []string{"reader"},
		})
		if err != nil {
			panic(err)
		}
		_, err = kp.CreatePermissionForResource(resp.ID, &uma.KcPermission{
			Name:        "writer-write-" + resp.ID,
			Description: "Writer can write user",
			Scopes:      []string{"write"},
			Roles:       []string{"writer"},
		})
		if err != nil {
			panic(err)
		}
		log.Printf("registered resources")
	})
	sm.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		stop()
	})
	sm.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	s := &http.Server{
		Addr:    "localhost:" + port,
		Handler: wrappInMiddleware(kp, rs, port, sm),
	}
	fmt.Println("listening...")
	if err := s.ListenAndServe(); err != nil {
		log.Printf("err: %v", err)
	}
}