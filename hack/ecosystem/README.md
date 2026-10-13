# Ecosystem submissions

Material for listings that are submitted from the maintainer's own accounts. Nothing here is built or
published by CI.

## istio.io integrations page

`istio-integration.md` is a draft page in the istio.io format.

1. Fork `istio/istio.io` and create `content/en/docs/ops/integrations/sealane/`.
2. Copy `istio-integration.md` there as `index.md`; bump `weight` if it collides with a sibling page.
3. Run `make lint` in the istio.io checkout (spelling, links and markdown rules of that repository) and fix
   what it reports; new product words go into its `.spelling` file.
4. Open a PR titled `Add Sealane to integrations` and sign the CLA if asked.

Acceptance is at the maintainers' discretion; third-party integrations are not guaranteed a page.

## CNCF Landscape

Files in `../landscape/`:

- `build-logo.py` regenerates `sealane-stacked.svg` (stacked mark + "Sealane" wordmark, text converted to
  paths, no background) with `python3 hack/landscape/build-logo.py`.
- `landscape-entry.yml` is the item to paste into `landscape.yml`.

Submit once the repository passes about 300 GitHub stars (the bar the Landscape maintainers expect for
non-CNCF projects):

1. Fork `cncf/landscape`.
2. Copy `hack/landscape/sealane-stacked.svg` to `hosted_logos/sealane.svg`.
3. Add the item from `landscape-entry.yml` to `landscape.yml` under "Orchestration & Management" →
   "Service Mesh", keeping the list alphabetical.
4. Open the PR and follow the bot's checks (logo and repository validation).
