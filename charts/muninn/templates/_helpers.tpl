{{/*
Computes the self-signed CA and serving certificate once per render,
reusing whatever's already in the cluster (via lookup) instead of rotating
it on every helm upgrade. genCA/genSignedCert are non-deterministic per
call, so without this, the Secret template and the
MutatingWebhookConfiguration's caBundle - two separate files - would each
generate their own, mismatched cert on a fresh install. Mutating the root
context via `set` is what makes the result visible to every template in
the same render, since Helm passes the same underlying map to each one.

lookup returns empty during `helm template`/--dry-run (no live API
connection), so those paths always fall back to generating fresh - that's
fine, since nothing there is actually applied to a cluster.
*/}}
{{- define "muninn.webhookCert" -}}
{{- if not $.muninnWebhookCert }}
{{- $secret := lookup "v1" "Secret" $.Release.Namespace $.Values.certificate.secretName }}
{{- if $secret }}
{{- $_ := set $ "muninnWebhookCert" (dict "cert" (index $secret.data "tls.crt") "key" (index $secret.data "tls.key") "ca" (index $secret.data "ca.crt")) }}
{{- else }}
{{- $altNames := list (printf "muninn-webhook.%s.svc" $.Release.Namespace) (printf "muninn-webhook.%s.svc.cluster.local" $.Release.Namespace) }}
{{- $ca := genCA "muninn-webhook-ca" 3650 }}
{{- $cert := genSignedCert "muninn-webhook" nil $altNames 3650 $ca }}
{{- $_ := set $ "muninnWebhookCert" (dict "cert" ($cert.Cert | b64enc) "key" ($cert.Key | b64enc) "ca" ($ca.Cert | b64enc)) }}
{{- end }}
{{- end }}
{{- end }}
