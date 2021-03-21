package auth

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/globals"
)

// Authenticate verifies a given OIDC ID-Token using the configured OIDC provider.
// It returns the user's id if verification was successful (or an empty string if no provider was configured).
// TODO: Currently, the userId is set to the "email" property of the claim, this could be made configurable. But: ensure that this is unique across the user base!
func Authenticate(idToken, oidcProvider string, cfg *config.Config) (string, error) {
	globals.AppLogger.Info("in authenticate")
	userId := ""
	if idToken == "" || len(cfg.OIDCConfigs) == 0 {
		return "", nil
	}
	globals.AppLogger.Debug("checking config")
	var oidcConf *config.OIDCConfig
	for _, c := range cfg.OIDCConfigs {
		if c.Name == oidcProvider {
			oidcConf = &c
			break
		}
	}
	if oidcConf == nil {
		globals.AppLogger.Debug("no oidc config found for provider", "provider", oidcProvider)
		return "", nil
	}
	globals.AppLogger.Debug("found oidc config")
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
		globals.AppLogger.Error("could not verify token", "error", err)
		return "", err
	}

	claims := struct {
		Sub string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
	}{}
	err = verifiedIdToken.Claims(&claims)
	if err != nil {
		globals.AppLogger.Error("could not unmarshal claims", "error", err)
		return "", err
	}
	userId = claims.Sub
	if claims.PreferredUsername != "" {
		userId = claims.PreferredUsername
	}
	return userId, nil
}
