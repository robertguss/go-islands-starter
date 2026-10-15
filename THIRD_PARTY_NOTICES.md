# Copied source notices

`components/`, `utils/shadcn-templ.go`, `web/shadcn-tailwind.css`, and `web/tw-animate.css` were installed with the shadcn-templ CLI from **github.com/axadrn/shadcn-templ/v2 v2.0.0-beta.15**, style `base-nova`, on 2026-10-15. The module's integrity is recorded in go.sum. The upstream MIT license is retained in [components/LICENSE](components/LICENSE).

A temporary local registry served that exact downloaded module's registry handlers and assets. No live upstream registry is needed to build. The installed roots are button, card, dialog, input, and textarea, plus their icon/base-ui dependencies. Unused roots were removed. The generated script manifest and templates are committed; the resulting browser bundles are built into web/dist and not committed.

The main Tailwind tokens and application styling in web/styles.css are locally customized. Imported component sources are otherwise retained. Review component changes and rerun browser, CSP, keyboard, and accessibility acceptance before updating this beta.

The icon component contains Lucide icon data. Its ISC notice and the MIT notice for icons derived from Feather are retained in [components/icon/LICENSE](components/icon/LICENSE).

Dependencies resolved through Go and npm retain their own licenses; this repository's MIT license does not replace those notices. Socket Firewall is a CI-only external tool and is not shipped in the application image.
