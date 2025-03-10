package rp

import (
	"context"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/pckhoi/uma/pkg/httputil"
	"github.com/pckhoi/uma/pkg/urlencode"
)

type KeycloakClient struct {
	oidc         *oidc.Provider
	clientID     string
	clientSecret string
	client       *http.Client
}

func NewKeycloakClient(issuer, clientID, clientSecret string, client *http.Client) (*KeycloakClient, error) {
	kc := &KeycloakClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		client:       client,
	}
	var err error
	kc.oidc, err = oidc.NewProvider(oidc.ClientContext(context.Background(), client), issuer)
	if err != nil {
		return nil, err
	}
	return kc, nil
}

type Credentials struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func (kc *KeycloakClient) Authenticate() (*httputil.ClientCreds, error) {
	resp, err := httputil.PostFormUrlencoded(kc.client, kc.oidc.Endpoint().TokenURL, nil, map[string][]string{
		"grant_type":    {"client_credentials"},
		"client_id":     {kc.clientID},
		"client_secret": {kc.clientSecret},
	})
	if err != nil {
		return nil, err
	}
	creds := &httputil.ClientCreds{}
	if err = httputil.DecodeJSONResponse(resp, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func (kc *KeycloakClient) AuthenticateUserWithPassword(username, password string) (creds *Credentials, err error) {
	params := map[string][]string{
		"grant_type":    {"password"},
		"client_id":     {kc.clientID},
		"client_secret": {kc.clientSecret},
		"scope":         {"openid"},
		"username":      {username},
		"password":      {password},
	}
	resp, err := httputil.PostFormUrlencoded(kc.client, kc.oidc.Endpoint().TokenURL, nil, params)
	if err != nil {
		return
	}
	tok := &Credentials{}
	if err = httputil.DecodeJSONResponse(resp, tok); err != nil {
		return
	}
	return tok, nil
}

func (kc *KeycloakClient) RefreshCredentials(creds Credentials) (*Credentials, error) {
	params := map[string][]string{
		"grant_type":    {"refresh_token"},
		"client_id":     {kc.clientID},
		"client_secret": {kc.clientSecret},
		"refresh_token": {creds.RefreshToken},
	}
	resp, err := httputil.PostFormUrlencoded(kc.client, kc.oidc.Endpoint().TokenURL, nil, params)
	if err != nil {
		return nil, err
	}
	tok := &Credentials{}
	if err = httputil.DecodeJSONResponse(resp, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

type ClaimTokenFormat string

const (
	// AccessTokenFormat indicates that the ClaimToken parameter references an access token
	AccessTokenFormat ClaimTokenFormat = "urn:ietf:params:oauth:token-type:jwt"

	// IDTokenFormat indicates that the ClaimToken parameter references an OpenID Connect ID Token.
	IDTokenFormat ClaimTokenFormat = "https://openid.net/specs/openid-connect-core-1_0.html#IDToken"
)

type RPTRequest struct {
	// Ticket is optional. The most recent permission ticket received by the client as part of the UMA authorization process.
	Ticket string

	// ClaimToken  is optional. A string representing additional claims that should be considered by the server when evaluating
	// permissions for the resource(s) and scope(s) being requested. This parameter allows clients to push claims to Keycloak.
	// For more details about all supported token formats see ClaimTokenFormat parameter.
	ClaimToken string

	// ClaimTokenFormat is optional. A string indicating the format of the token specified in the ClaimToken parameter.
	// Inspect AccessTokenFormat and IDTokenFormat to learn more.
	ClaimTokenFormat ClaimTokenFormat

	// RPT is optional. A previously issued RPT which permissions should also be evaluated and added in a new one. This parameter
	// allows clients in possession of an RPT to perform incremental authorization where permissions are added on demand.
	RPT string `url:"rpt"`

	// Permission is optional. A string representing a set of one or more resources and scopes the client is seeking access.
	// This parameter can be defined multiple times in order to request permission for multiple resource and scopes.
	// This parameter is an extension to urn:ietf:params:oauth:grant-type:uma-ticket grant type in order to allow clients to
	// send authorization requests without a permission ticket. The format of the string must be: RESOURCE_ID#SCOPE_ID. For
	// instance: Resource A#Scope A, Resource A#Scope A, Scope B, Scope C, Resource A, #Scope A.
	Permission []string

	// Audience is optional. The client identifier of the resource server to which the client is seeking access. This parameter
	// is mandatory in case the permission parameter is defined. It serves as a hint to Keycloak to indicate the context in
	// which permissions should be evaluated.
	Audience string

	// ResponseIncludeResourceName is optional. A boolean value indicating to the server whether resource names should be included
	// in the RPT’s permissions. If false, only the resource identifier is included.
	ResponseIncludeResourceName bool

	// ResponsePermissionsLimit is optional. An integer N that defines a limit for the amount of permissions an RPT can have. When
	// used together with rpt parameter, only the last N requested permissions will be kept in the RPT.
	ResponsePermissionsLimit int

	// SubmitRequest is optional. A boolean value indicating whether the server should create permission requests to the resources
	// and scopes referenced by a permission ticket. This parameter only has effect if used together with the ticket parameter as
	// part of a UMA authorization process.
	SubmitRequest bool
}

func (kc *KeycloakClient) RequestRPT(accessToken string, request RPTRequest) (rpt string, err error) {
	values, err := urlencode.ToValues(request)
	if err != nil {
		return "", err
	}
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:uma-ticket")
	resp, err := httputil.PostFormUrlencoded(kc.client, kc.oidc.Endpoint().TokenURL, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+accessToken)
	}, *values)
	if err != nil {
		return "", err
	}
	tok := &Credentials{}
	if err := httputil.DecodeJSONResponse(resp, tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}
