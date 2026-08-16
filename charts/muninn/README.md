# muninn

![Version: 0.2.4](https://img.shields.io/badge/Version-0.2.4-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.2.2](https://img.shields.io/badge/AppVersion-0.2.2-informational?style=flat-square)

Kubernetes-native runtime configuration resolver

**Homepage:** <https://github.com/Garoze/Muninn>

## Source Code

* <https://github.com/Garoze/Muninn>

## Requirements

Kubernetes: `>=1.27.0-0`

| Repository | Name | Version |
|------------|------|---------|
| https://charts.jetstack.io | cert-manager | v1.21.1 |
| https://kubernetes-sigs.github.io/secrets-store-csi-driver/charts | secrets-store-csi-driver | 1.6.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.repository | string | `"ghcr.io/garoze/muninn"` | Repository holding the resolver and webhook image. Both roles run the same image, distinguished by the subcommand they are started with. |
| image.tag | string | `"latest"` | Image tag. The default is the floating tag, which moves with every official release. |
| image.digest | string | `""` | Digest of the exact image to run, for an install that must match an artifact that was verified. A tag can be moved to point at different content after it was checked, and the default here is the floating one - so a consumer who verified a digest and then installed by tag has verified something other than what runs. Set alongside the tag rather than instead of it: the digest resolves, the tag stays readable. See https://github.com/Garoze/Muninn/blob/main/docs/verification.md |
| webhook.enabled | bool | `true` | Gates every webhook-* template, including the Certificate/Issuer - false is what a fresh-cluster two-phase install needs for its first pass, when cert-manager.enabled is also being turned on for the first time and its own webhook isn't serving yet. See cert-manager.enabled's comment below for the full two-phase sequence. |
| webhook.failurePolicy | string | `"Fail"` | Fail blocks Pod creation wherever the webhook applies whenever it's unavailable; Ignore makes injection silently best-effort instead - a cluster operator's risk tolerance to set, not something to hardcode. |
| webhook.excludedNamespaces | list | `[]` | Namespaces the webhook never applies to. Always unioned with kube-system and .Release.Namespace, never a replacement for them - dropping either reintroduces the deadlock where an unavailable webhook blocks its own replacement Pod from scheduling. |
| certificate.mode | string | `"cert-manager"` | Where the webhook's serving certificate comes from. cert-manager: today's verified behavior; requires cert-manager already installed (external prerequisite, not managed by this chart). self-signed: the chart generates a CA and serving cert itself - zero prerequisites, but see webhook-certificate-selfsigned.yaml for the rotation hazard this carries on every helm upgrade. provided: bring your own PKI - supply an existing Secret and CA bundle. |
| certificate.secretName | string | `"muninn-webhook-tls"` | The Secret holding the serving cert - consistent across all three modes. cert-manager writes it, self-signed generates it, provided expects it to already exist under this name. |
| certificate.provided.caBundle | string | `""` | PEM-encoded CA bundle the API server should trust, base64 encoded. Required by, and only read in, the provided mode. |
| secrets.enabled | bool | `false` | Grants the webhook what secret delivery needs. Off by default: the Create mode below needs a create/patch grant on SecretProviderClass in arbitrary consumer namespaces, which is exactly what Reference mode exists to avoid - it must not be granted unasked. |
| secrets.spcMode | string | `"Create"` | Create: the webhook generates the SecretProviderClass itself, needing create/patch RBAC. Reference: a pre-provisioned SecretProviderClass is expected and only validated, needing get RBAC alone. |
| secrets.vault.address | string | `"http://vault.kube-system:8200"` | Address the CSI driver's Vault provider reaches Vault on. Read by the generated SecretProviderClass, never by the resolver itself. |
| secrets.vault.roleName | string | `"muninn"` | Vault role the driver authenticates as when fetching a referenced secret. |
| cert-manager.enabled | bool | `false` | Installs cert-manager itself as a dependency. Off by default - the verified configuration is a cluster that already has cert-manager installed separately (the common case), which needs nothing here at all. Two things to know before turning this on. Subcharts install into this release's namespace, not their own conventional one - cert-manager normally lives in a `cert-manager` namespace; as a dependency here it lands in .Release.Namespace instead. It still works (cert-manager has no semantic dependency on its own namespace name), but a cluster whose cert-manager arrived this way has it somewhere unexpected. `helm uninstall` also then removes cert-manager along with everything else on the cluster that came to depend on it - default-off contains that to whoever explicitly opted in. And a fresh cluster needs a two-phase install: cert-manager's own webhook must be serving before Muninn's own Certificate can be admitted, and Helm's install ordering has no concept of that dependency between two separate charts. `helm install muninn ./charts/muninn --set cert-manager.enabled=true --set webhook.enabled=false --wait` then `helm upgrade muninn ./charts/muninn --set cert-manager.enabled=true`. A single-pass install with webhook.enabled=true races cert-manager's own webhook coming up and fails; running it as two commands isn't optional on a fresh cluster. |
| cert-manager.crds.enabled | bool | `true` | Installs cert-manager's own custom resource definitions with it. Helm never upgrades definitions installed this way. |
| secrets-store-csi-driver.enabled | bool | `false` | Installs secrets-store-csi-driver as a dependency. Off by default - a cluster that already runs the driver gets nothing extra. Same subchart-namespace caveat as cert-manager above, but the ordering hazard is milder: nothing at install time depends on the driver being ready, since the webhook only touches a SecretProviderClass at admission, for namespaces whose config actually carries secret references - a driver still starting up when the chart finishes installing causes no install failure, only a wait for the first affected Pod. Vault itself is never installed by this chart, on or off - it's a stateful secrets backend the operator owns, not infrastructure Muninn integrates with. See test/e2e/csi_e2e_test.go for a working dev-mode topology reference. |
