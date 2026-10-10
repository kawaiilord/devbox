# Client credential storage and signed updates

## Session credentials

Access and refresh tokens are persisted through `flutter_secure_storage`, not
preferences or application files. Windows uses Credential Manager, Apple
platforms use Keychain, Android uses encrypted storage backed by Keystore, and
Linux uses Secret Service/libsecret. Token rotation writes the new access and
refresh pair immediately through the same store. Logout and failed session
restoration delete the stored entry.

Linux packages require a Secret Service implementation such as GNOME Keyring.
Web builds require a secure HTTPS context; browser storage cannot provide the
same OS-account isolation as desktop credential managers, so high-risk shared
browser deployments should disable persistent browser profiles.

## Update trust model

Build clients with two compile-time values:

```text
--dart-define=UPDATE_MANIFEST_URL=https://updates.example.com/windows.json
--dart-define=UPDATE_PUBLIC_KEY=<base64 raw 32-byte Ed25519 public key>
```

The client accepts a manifest only when:

- the manifest and package URLs use HTTPS;
- its platform exactly matches the running platform;
- the detached Ed25519 signature is valid;
- its version is newer than the installed semantic version;
- the package is at most 2 GiB and its received size matches exactly;
- the downloaded package SHA-256 equals the signed digest.

The signed UTF-8 payload is six newline-separated fields with no trailing
newline:

```text
version\nplatform\nurl\nsha256\nsize\npublished_at
```

Desktop downloads are first written with a `.partial` suffix. Only a fully
verified package is renamed to `.verified`; failed downloads are deleted. The
application does not silently execute installers.

## Signing releases

Generate an Ed25519 key offline and keep the private key only in the release
signing environment. The CI artifact `sameframe-update-signer-linux` signs a
package using the private key from an environment variable:

```bash
SAMEFRAME_UPDATE_PRIVATE_KEY='<base64 private key>' sign-update \
  -version 1.2.0 -platform windows \
  -url https://updates.example.com/sameframe-1.2.0.zip \
  -package sameframe-1.2.0.zip -output windows.json
```

Publish the immutable package before its manifest. Never reuse a version for
different bytes. The public key is pinned into clients; key rotation therefore
requires a release signed by the currently trusted key or a multi-key migration
implemented before the old key is retired.
