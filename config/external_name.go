package config

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// ExternalNameConfigs contains all external name configurations for this
// provider.
var ExternalNameConfigs = map[string]config.ExternalName{
	// Organizations
	"gitea_org": config.IdentifierFromProvider,

	// Teams and membership
	"gitea_team":            config.IdentifierFromProvider,
	"gitea_team_membership": config.IdentifierFromProvider,
	"gitea_team_members":    config.IdentifierFromProvider,

	// Users
	"gitea_user": config.IdentifierFromProvider,

	// Repositories
	"gitea_repository":                  config.IdentifierFromProvider,
	"gitea_repository_key":              config.IdentifierFromProvider,
	"gitea_repository_webhook":          config.IdentifierFromProvider,
	"gitea_repository_branch_protection": config.IdentifierFromProvider,
	"gitea_repository_actions_secret":   config.IdentifierFromProvider,
	"gitea_repository_actions_variable": config.IdentifierFromProvider,

	// Tokens and keys
	"gitea_token":      config.IdentifierFromProvider,
	"gitea_public_key": config.IdentifierFromProvider,
	"gitea_gpg_key":    config.IdentifierFromProvider,

	// Other resources
	"gitea_fork":       config.IdentifierFromProvider,
	"gitea_git_hook":   config.IdentifierFromProvider,
	"gitea_oauth2_app": config.IdentifierFromProvider,
}

func idWithStub() config.ExternalName {
	e := config.IdentifierFromProvider
	e.GetExternalNameFn = func(tfstate map[string]any) (string, error) {
		en, _ := config.IDAsExternalName(tfstate)
		return en, nil
	}
	return e
}

// ExternalNameConfigurations applies all external name configs listed in the
// table ExternalNameConfigs and sets the version of those resources to v1beta1
// assuming they will be tested.
func ExternalNameConfigurations() config.ResourceOption {
	return func(r *config.Resource) {
		if e, ok := ExternalNameConfigs[r.Name]; ok {
			r.ExternalName = e
		}
	}
}

// ExternalNameConfigured returns the list of all resources whose external name
// is configured manually.
func ExternalNameConfigured() []string {
	l := make([]string, len(ExternalNameConfigs))
	i := 0
	for name := range ExternalNameConfigs {
		// $ is added to match the exact string since the format is regex.
		l[i] = name + "$"
		i++
	}
	return l
}
