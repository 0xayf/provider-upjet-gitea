package clients

import (
	"context"
	"encoding/json"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiscluster "github.com/0xayf/provider-upjet-gitea/apis/cluster"
	clustergitea "github.com/0xayf/provider-upjet-gitea/apis/cluster/gitea/v1alpha1"
	clusterv1beta1 "github.com/0xayf/provider-upjet-gitea/apis/cluster/v1beta1"
	apisnamespaced "github.com/0xayf/provider-upjet-gitea/apis/namespaced"
	namespacedgitea "github.com/0xayf/provider-upjet-gitea/apis/namespaced/gitea/v1alpha1"
	namespacedv1beta1 "github.com/0xayf/provider-upjet-gitea/apis/namespaced/v1beta1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := apiscluster.AddToScheme(s); err != nil {
		t.Fatalf("add cluster scheme: %v", err)
	}
	if err := apisnamespaced.AddToScheme(s); err != nil {
		t.Fatalf("add namespaced scheme: %v", err)
	}
	return s
}

func TestResolveLegacyProviderConfig(t *testing.T) {
	s := testScheme(t)

	pc := &clusterv1beta1.ProviderConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: clusterv1beta1.SchemeGroupVersion.String(), Kind: "ProviderConfig"},
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec: clusterv1beta1.ProviderConfigSpec{
			Credentials: clusterv1beta1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{SecretReference: xpv1.SecretReference{Name: "provider-secret", Namespace: "crossplane-system"}, Key: "credentials"},
				},
			},
		},
	}

	mg := &clustergitea.Repository{ObjectMeta: metav1.ObjectMeta{Name: "repo", UID: ktypes.UID("repo-legacy-uid")}}
	mg.SetProviderConfigReference(&xpv1.Reference{Name: "default"})

	c := ctrlclientfake.NewClientBuilder().WithScheme(s).WithObjects(pc).Build()

	got, err := resolveProviderConfig(context.Background(), c, mg)
	if err != nil {
		t.Fatalf("resolve legacy provider config: %v", err)
	}
	if got == nil || got.Credentials.SecretRef == nil {
		t.Fatalf("expected resolved credentials secret ref")
	}
	if got.Credentials.SecretRef.Name != "provider-secret" {
		t.Fatalf("unexpected secret name: %s", got.Credentials.SecretRef.Name)
	}
	if got.Credentials.SecretRef.Namespace != "crossplane-system" {
		t.Fatalf("unexpected secret namespace: %s", got.Credentials.SecretRef.Namespace)
	}
}

func TestResolveModernNamespacedProviderConfigPreservesSecretNamespace(t *testing.T) {
	s := testScheme(t)

	pc := &namespacedv1beta1.ProviderConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: namespacedv1beta1.SchemeGroupVersion.String(), Kind: "ProviderConfig"},
		ObjectMeta: metav1.ObjectMeta{Name: "app-pc", Namespace: "crossplane-examples"},
		Spec: namespacedv1beta1.ProviderConfigSpec{
			Credentials: namespacedv1beta1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{SecretReference: xpv1.SecretReference{Name: "crossplane-gitea-token", Namespace: "gitea"}, Key: "credentials"},
				},
			},
		},
	}

	mg := &namespacedgitea.Repository{ObjectMeta: metav1.ObjectMeta{Name: "repo", Namespace: "crossplane-examples", UID: ktypes.UID("repo-ns-uid")}}
	mg.SetProviderConfigReference(&xpv1.ProviderConfigReference{Name: "app-pc", Kind: namespacedv1beta1.ProviderConfigKind})

	c := ctrlclientfake.NewClientBuilder().WithScheme(s).WithObjects(pc).Build()

	got, err := resolveProviderConfig(context.Background(), c, mg)
	if err != nil {
		t.Fatalf("resolve modern namespaced provider config: %v", err)
	}
	if got == nil || got.Credentials.SecretRef == nil {
		t.Fatalf("expected resolved credentials secret ref")
	}
	if got.Credentials.SecretRef.Namespace != "gitea" {
		t.Fatalf("expected secret namespace gitea, got %s", got.Credentials.SecretRef.Namespace)
	}
}

func TestResolveModernClusterProviderConfig(t *testing.T) {
	s := testScheme(t)

	pc := &namespacedv1beta1.ClusterProviderConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: namespacedv1beta1.SchemeGroupVersion.String(), Kind: "ClusterProviderConfig"},
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-pc"},
		Spec: namespacedv1beta1.ProviderConfigSpec{
			Credentials: namespacedv1beta1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{SecretReference: xpv1.SecretReference{Name: "crossplane-gitea-token", Namespace: "gitea"}, Key: "credentials"},
				},
			},
		},
	}

	mg := &namespacedgitea.Repository{ObjectMeta: metav1.ObjectMeta{Name: "repo", UID: ktypes.UID("repo-cluster-uid")}}
	mg.SetProviderConfigReference(&xpv1.ProviderConfigReference{Name: "cluster-pc", Kind: namespacedv1beta1.ClusterProviderConfigKind})

	c := ctrlclientfake.NewClientBuilder().WithScheme(s).WithObjects(pc).Build()

	got, err := resolveProviderConfig(context.Background(), c, mg)
	if err != nil {
		t.Fatalf("resolve modern cluster provider config: %v", err)
	}
	if got == nil {
		t.Fatalf("expected provider config spec")
	}
	if got.Credentials.Source != xpv1.CredentialsSourceSecret {
		t.Fatalf("unexpected credential source: %s", got.Credentials.Source)
	}
}

func TestTerraformSetupBuilderUsesTokenCredentials(t *testing.T) {
	s := testScheme(t)

	credentials := map[string]string{
		"base_url": "http://gitea-http.gitea.svc.cluster.local:3000",
		"token":    "test-token",
	}
	rawCredentials, err := json.Marshal(credentials)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "crossplane-gitea-token", Namespace: "gitea"},
		Data: map[string][]byte{
			"credentials": rawCredentials,
		},
	}

	pc := &namespacedv1beta1.ProviderConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: namespacedv1beta1.SchemeGroupVersion.String(), Kind: "ProviderConfig"},
		ObjectMeta: metav1.ObjectMeta{Name: "app-pc", Namespace: "crossplane-examples"},
		Spec: namespacedv1beta1.ProviderConfigSpec{
			Credentials: namespacedv1beta1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{SecretReference: xpv1.SecretReference{Name: "crossplane-gitea-token", Namespace: "gitea"}, Key: "credentials"},
				},
			},
		},
	}

	mg := &namespacedgitea.Repository{ObjectMeta: metav1.ObjectMeta{Name: "repo", Namespace: "crossplane-examples", UID: ktypes.UID("repo-setup-uid")}}
	mg.SetProviderConfigReference(&xpv1.ProviderConfigReference{Name: "app-pc", Kind: namespacedv1beta1.ProviderConfigKind})

	c := ctrlclientfake.NewClientBuilder().WithScheme(s).WithObjects(pc, secret).Build()

	setupFn := TerraformSetupBuilder("v1", "registry.terraform.io/go-gitea/gitea", "0.7.0")
	got, err := setupFn(context.Background(), c, mg)
	if err != nil {
		t.Fatalf("build terraform setup: %v", err)
	}

	if got.Configuration["base_url"] != credentials["base_url"] {
		t.Fatalf("unexpected base_url: %v", got.Configuration["base_url"])
	}
	if got.Configuration["token"] != credentials["token"] {
		t.Fatalf("unexpected token: %v", got.Configuration["token"])
	}
	if _, found := got.Configuration["username"]; found {
		t.Fatalf("did not expect username when token is provided")
	}
	if _, found := got.Configuration["password"]; found {
		t.Fatalf("did not expect password when token is provided")
	}
}
