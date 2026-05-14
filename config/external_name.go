package config

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// ExternalNameConfigs contains all external name configurations for this
// provider.
var ExternalNameConfigs = map[string]config.ExternalName{
	// Organizations and Users. External-name maps to the URL-key on the
	// Gitea API (/orgs/{name} and /users/{username}) so a single XR
	// claim can both adopt an existing org/user and create one on a
	// fresh cluster — neither possible when external-name is the
	// numeric ID Gitea assigns on create.
	"gitea_org":  config.ParameterAsIdentifier("name"),
	"gitea_user": config.ParameterAsIdentifier("username"),

	// Teams and membership are managed by hand-written controllers in
	// internal/controller/{cluster,namespaced}/gitea/team and
	// internal/controller/{cluster,namespaced}/team/membership. They are
	// intentionally absent from this map so upjet does not regenerate
	// Terraform-backed shells for them. The hand-written controllers
	// drive Gitea's per-unit permission model directly via the API.
	"gitea_team_members": config.IdentifierFromProvider,

	// Repositories. gitea_repository_actions_secret is managed by a
	// hand-written controller in internal/controller/{cluster,namespaced}/repository/actionssecret
	// to detect value rotation via a SHA-256 hash of the source secret;
	// it's intentionally absent from this map so upjet does not
	// regenerate a Terraform-backed shell for it.
	"gitea_repository":                  config.IdentifierFromProvider,
	"gitea_repository_key":              config.IdentifierFromProvider,
	"gitea_repository_webhook":          config.IdentifierFromProvider,
	"gitea_repository_branch_protection": config.IdentifierFromProvider,
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
