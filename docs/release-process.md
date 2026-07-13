# Release process

1. Start from a clean signed tag on a commit whose Windows reliability jobs, race suite, vet, vulnerability scan, and hardware evidence all pass.
2. Run `scripts/reliability-evidence.ps1` on the production hardware with a 24-hour soak source. Preserve `evidence.json` and its logs.
3. Run `scripts/package-release.ps1 -Version vX.Y.Z -ReliabilityEvidence <evidence.json> -CertificatePath <certificate.pfx> -RequireSigning` on the clean Windows release worker.
4. Verify both Authenticode signatures, `SHA256SUMS.txt`, `SBOM.spdx.json`, `release-manifest.json`, embedded manifest/DPI behaviour, and that the build identity shown in the window/log matches the tag and commit.
5. Extract the package on a clean supported Windows image. Run `install.ps1`, launch through `cusus-supervisor.exe`, open a version-1 fixture, save a copy, and complete the pre-show checklist. Install the candidate over the test install, then exercise `install.ps1 -Rollback` and reopen the last-good show.
6. Publish the immutable zip, hashes, SBOM, release manifest, operator release notes, and reliability evidence together. Never replace an artifact under an existing version.

The build uses `-trimpath`, disables VCS auto-stamping, clears the Go linker build ID, injects version/commit/UTC commit time, normalises staged file times to the commit timestamp, and records every module in SPDX 2.3 output. Rebuilding the same tag with the same Go toolchain and signing policy should reproduce the unsigned payload; Authenticode timestamps intentionally make signed bytes unique.
