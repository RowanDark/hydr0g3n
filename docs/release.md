# Release process

The project publishes releases using [GoReleaser](https://goreleaser.com/). The GitHub Actions workflow under `.github/workflows/release.yml` is triggered whenever a tag that matches `v*.*.*` is pushed. The workflow builds the binaries for all supported platforms and uploads them as release artifacts.

## Optional GPG signing

Releases can optionally be signed with a GPG key. When signing is enabled, GoReleaser will upload detached ASCII-armored signatures (`.asc`) for both the generated archives and the checksum file.

To allow the workflow to sign release artifacts:

1. Export the GPG private key you want to use for signing:
   ```bash
   gpg --export-secret-keys --armor YOUR_KEY_ID > private-key.asc
   ```
2. Add the exported key to the repository secrets as `GPG_PRIVATE_KEY`. You can copy the contents of `private-key.asc` directly into the secret — no additional encoding is required.
3. If the key is protected with a passphrase, store it in the `GPG_PASSPHRASE` secret. Leave this secret unset if your key does not have a passphrase.

When both secrets are configured, the release workflow will import the private key during the run and use it to sign every archive as well as the checksum file. If the secrets are not defined, the signing step is skipped and the workflow behaves exactly as it did before.
