# Contributing to SerpentSeek

Thank you for considering a contribution! By participating you agree to abide
by the [Code of Conduct](CODE_OF_CONDUCT.md).

## License of Contributions ("Inbound = Outbound")

SerpentSeek is licensed under the **GNU General Public License v3.0**
(GPL-3.0-only, [LICENSE](LICENSE)). Any contribution you submit (code, docs,
configuration) is distributed under the same license. You retain your own
copyright; no copyright assignment is required. On your first Pull Request,
comment **"I have read the CLA Document and I hereby sign the CLA"** to accept
[CLA.md](CLA.md) — it grants the maintainer a broad license to your
contributions, **including relicensing rights (CLA §8)**, but transfers no
copyright.

Because the project embeds the web UI into a single binary, contributions to
`web/` are also licensed GPLv3.

## Developer Certificate of Origin (DCO)

Every commit must be signed off to certify that you wrote it (or otherwise
have the right to submit it) and that you license it under GPLv3. Add a
`Signed-off-by` line to each commit message:

```
Signed-off-by: Your Name <your@email.example>
```

`git commit -s` adds this automatically.

By signing off you certify the Developer Certificate of Origin, version 1.1:

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

## Practical Notes

- Do not commit secrets: `.env`, API keys, database files (`*.db`) are
  git-ignored — keep it that way. Credentials must stay maskable by the
  logger's redaction registry.
- Third-party code: only Apache-2.0 / MIT / BSD / ISC compatible material may
  be vendored. When you add or bump a Go or npm dependency, regenerate the
  attribution file and include the change in your PR:

  ```bash
  scripts/gen-third-party-licenses.sh > THIRD_PARTY_LICENSES.md
  ```

- Run `go build ./...`, `go test ./...` and `gofmt` before submitting
  Go changes; `npm run check` for the web UI.

## Scope

The `main` branch is GPLv3 and copyleft. If you need a piece of this codebase
under a different license (e.g. to reuse a provider driver in a non-GPL
project), open an issue to discuss with the maintainers first.
