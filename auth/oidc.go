package auth

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/tcriess/lightspeed-chat/config"
)

/*
Authenticate verifies a given OIDC ID-Token using the configured OIDC provider.
It returns the user's id if verification was successful (or an empty string if no provider was configured).
TODO: Currently, the userId is set to the "email" property of the claim, this could be made configurable. But: ensure that this is unique across the userbase!
*/
func Authenticate(idToken, oidcProvider string, cfg *config.Config) (string, error) {
	userId := ""
	if idToken == "" || len(cfg.OIDCConfigs) == 0 {
		return "", nil
	}
	var oidcConf *config.OIDCConfig
	for _, c := range cfg.OIDCConfigs {
		if c.Name == oidcProvider {
			oidcConf = &c
		}
	}
	if oidcConf == nil {
		return "", nil
	}
	provider, err := oidc.NewProvider(context.Background(), oidcConf.ProviderUrl)
	if err != nil {
		return "", err
	}
	conf := oidc.Config{}
	if oidcConf.ClientId == "" {
		conf.SkipClientIDCheck = true
	} else {
		conf.ClientID = oidcConf.ClientId
	}
	verifier := provider.Verifier(&conf)
	verifiedIdToken, err := verifier.Verify(context.Background(), idToken)
	if err != nil {
		return "", err
	}

	claims := struct {
		Email string `json:"email"`
	}{}
	err = verifiedIdToken.Claims(&claims)
	if err != nil {
		return "", err
	}
	if claims.Email != "" {
		userId = claims.Email
	}
	return userId, nil
}
