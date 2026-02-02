// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	hook "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/git/hook"
	fork "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/gitea/fork"
	org "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/gitea/org"
	repository "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/gitea/repository"
	team "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/gitea/team"
	token "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/gitea/token"
	user "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/gitea/user"
	key "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/gpg/key"
	app "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/oauth2/app"
	providerconfig "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/providerconfig"
	keypublic "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/public/key"
	actionssecret "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/repository/actionssecret"
	actionsvariable "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/repository/actionsvariable"
	branchprotection "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/repository/branchprotection"
	keyrepository "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/repository/key"
	webhook "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/repository/webhook"
	members "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/team/members"
	membership "github.com/0xayf/provider-upjet-gitea/internal/controller/namespaced/team/membership"
)

// Setup creates all controllers with the supplied logger and adds them to
// the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		hook.Setup,
		fork.Setup,
		org.Setup,
		repository.Setup,
		team.Setup,
		token.Setup,
		user.Setup,
		key.Setup,
		app.Setup,
		providerconfig.Setup,
		keypublic.Setup,
		actionssecret.Setup,
		actionsvariable.Setup,
		branchprotection.Setup,
		keyrepository.Setup,
		webhook.Setup,
		members.Setup,
		membership.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated creates all controllers with the supplied logger and adds them to
// the supplied manager gated.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		hook.SetupGated,
		fork.SetupGated,
		org.SetupGated,
		repository.SetupGated,
		team.SetupGated,
		token.SetupGated,
		user.SetupGated,
		key.SetupGated,
		app.SetupGated,
		providerconfig.SetupGated,
		keypublic.SetupGated,
		actionssecret.SetupGated,
		actionsvariable.SetupGated,
		branchprotection.SetupGated,
		keyrepository.SetupGated,
		webhook.SetupGated,
		members.SetupGated,
		membership.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
