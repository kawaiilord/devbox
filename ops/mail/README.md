# SameFrame self-hosted mail

This deployment uses the owner's Debian server at `38.76.190.179` for direct SMTP
delivery. It does not use a commercial relay or any other project's credentials.
Sender: `no-reply@sameframe.top`. SMTP account secrets and DKIM private keys stay
on that server and must never be committed.

## Preparation

Copy this directory to `/opt/sameframe/mail-ops`, then run `python3 install.py` as
root. The installer requires the existing SameFrame site certificate and local
administrator credential file. It backs up the previous SMTP configuration,
affected host files and PostgreSQL, then installs Debian Postfix, OpenDKIM and
Cyrus SASL. No application rebuild or database migration is needed.

If an unrelated package job holds the dpkg lock, the installer waits up to five
minutes. An interrupted installation before credential creation can resume with
`python3 install.py --resume` using the original backup.

The installer sets up:

- Port 587: STARTTLS and a random password, with sender ownership checks.
- Port 25: accepts only local-domain recipients, never unauthenticated relay.
- A 2048-bit DKIM key, selector `sf202610`.
- Local postmaster/abuse/no-reply mail delivery into `/var/lib/sameframe-mail/Maildir`.
- A temporary submission rule that refuses external recipients while setup is pending.
- An encrypted SMTP configuration in SameFrame, with sending **disabled**.
- TLS renewal reload and bounded log rotation.

The temporary submission hostname is `sameframe.top`, matching the existing TLS
certificate. Activation obtains a certificate for `mail.sameframe.top` and moves
submission to that dedicated DNS-only hostname.

## Required external setup

Import the generated DNS records in `/opt/sameframe/mail/dns-records.json` into
the `sameframe.top` Cloudflare zone. The accompanying release report includes a
BIND-format import file. Keep `mail.sameframe.top` **DNS only**, without the
Cloudflare proxy. Check existing SPF/MX records before importing; do not create
duplicate SPF records or overwrite another mailbox service.

Ask the server provider to allow outbound TCP 25 and set:

`38.76.190.179 -> mail.sameframe.top` (PTR/rDNS).

Forward and reverse DNS must agree. Gmail also requires sender authentication
and TLS: [Google sender requirements](https://support.google.com/mail/answer/81126).
Cloudflare explains the DNS records and mail-host proxy setting in its
[email DNS guide](https://developers.cloudflare.com/dns/manage-dns-records/how-to/email-records/).

## Verification and activation

1. `python3 validate.py`: tests TLS, authentication, rejection of unauthorized
   relay/senders, the temporary delivery gate, and a local DKIM-signed message
   submitted from the deployed app's network. No external email is sent.
2. `python3 check.py`: verifies DNS/PTR and direct port 25 connectivity. It exits
   nonzero until all prerequisites pass.
3. `python3 activate.py --test-to USER_AUTHORIZED_EMAIL`: checks prerequisites,
   obtains the mail-host certificate, opens authenticated external submission,
   enables SMTP in SameFrame and queues one test email.
4. Inspect `/var/log/sameframe-mail.log` and `postqueue -p`; confirm the recipient
   received the email. SMTP queue acceptance alone is not proof of delivery or
   inbox placement. Then test the registration email flow with the owner.

An operational message can be inspected locally without putting its body or
verification code in a release report. The activation helper records a pending
delivery check and does not claim inbox success.

## Rollback

`python3 /opt/sameframe/mail-ops/install.py --rollback` verifies backup hashes,
restores the previous application SMTP configuration and TLS renewal script,
and disables the new mail services. It keeps package files, DKIM keys, queues
and the local mailbox for review. It does not restore the database or undo new
account activity. DNS/PTR changes, if subsequently made, need their own reversal.

Implementation references: [Postfix settings](https://manpages.debian.org/trixie/postfix/postconf.5.en.html),
[OpenDKIM settings](https://manpages.debian.org/trixie/opendkim/opendkim.conf.5.en.html).
