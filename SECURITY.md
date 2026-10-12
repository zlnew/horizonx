# Security model and trust boundaries

What HorizonX protects, and — more usefully — what it does not. Read this before
giving HorizonX credentials or exposing it beyond your own machine.

## The one boundary that matters

**Control-plane access is equivalent to root on every agent host.**

The agent does not run as root (see below), but it holds access to the docker
socket, and `docker` group membership is host-root-equivalent by construction:
anything that can talk to `docker.sock` can start a container that mounts the
host filesystem and take the host. No amount of unit hardening changes that,
because the socket *is* the capability.

So the boundary that matters sits upstream of the agent:

```
who can reach the control plane
  ~= who can enqueue a deploy job
  ~= root on every host running an agent
```

Treat a HorizonX login with deploy rights, the server API, the agent token, and
the server's database as root-equivalent secrets for every app host. Do not
point HorizonX at hosts you would not let a deploy-capable user own.

## How the agent actually runs

Both installers (`horizonx install agent`, `scripts/install-agent.sh`) create a
dedicated, unprivileged system account and put it in the `docker` group:

```
useradd -r -g horizonx -s /usr/sbin/nologin horizonx
usermod -aG docker horizonx
```

and the systemd unit (`internal/app/templates/horizonx-agent.service`) adds:

```ini
User=horizonx
Group=horizonx
SupplementaryGroups=docker        # the socket — i.e. root-equivalent
NoNewPrivileges=true
ProtectSystem=strict              # everything read-only except:
ReadWritePaths=/var/lib/horizonx  # the apps workdir
ProtectHome=true
PrivateTmp=true
```

**What this buys:** the agent process has no root, no login shell, and no write
access outside its workdir. **What it does not buy:** any reduction in the
capability reachable through the docker socket.

`ReadWritePaths` deliberately excludes `/etc/horizonx`. That directory holds
`agent.env` (the agent's own API token) and, on single-box installs, the
server's `.env` (`DATABASE_URL`, `JWT_SECRET`, the encryption key). The agent
only ever *reads* its env at start — the installers write it under `sudo` — so
write access there would let a compromised agent rewrite its own credentials,
or the server's. If you tighten or extend this unit, keep that distinction.

## User input reaches command argv

Application configuration is user-controlled and ends up on agent argv, so
argument injection is remote code execution on a host the agent effectively
controls.

`RepoURL` was passed positionally to `git clone`, where git read a value
beginning with `-` as an option: `--upload-pack=<cmd>` was accepted and
executed. That is fixed — `--` is now passed before the positional arguments,
and branch names are validated separately. Two things to preserve:

- Do **not** remove those terminators.
- Do **not** treat `--` as a general remedy. Whether it is valid, and whether it
  changes meaning, depends on the command. `git checkout -f -- <branch>` treats
  the branch as a *pathspec* and breaks the pull path, which is why branch
  validation was used there instead.

## Release integrity

`install.sh` and the Go install/upgrade paths verify a minisign signature over
`SHA256SUMS` when `HORIZONX_RELEASE_PUBKEY` is set, and fail closed if the
signature is missing, malformed, or wrong.

With that variable **unset**, trust is TLS-only: whoever can serve or intercept
the release can substitute both the tarball and its checksum. Set it for
anything but a throwaway box.

A pinned key gives integrity and key provenance, **not** freshness. An attacker
who can serve an *older* validly signed release can still do so; there is no
rollback protection.

## Secrets at rest

Environment-variable values are AES-256-GCM encrypted in the database, keyed by
`ENV_ENCRYPTION_KEY`. With that variable unset they are keyed by
`SHA256(JWT_SECRET)`, so rotating `JWT_SECRET` orphans every stored secret.

Set `ENV_ENCRYPTION_KEY` to decouple the two. Existing rows stay readable
through the old JWT-derived key and are re-encrypted under the new key as they
are read.

**There is no key ring.** Rotating `ENV_ENCRYPTION_KEY` after it is in use will
orphan data. Keep it backed up and stable.

## Network exposure

- `/metrics` requires authentication (`metrics_read`); it is not a public
  scrape target.
- Bind the API to `127.0.0.1` and front it with `tailscale serve`. Binding it
  non-loopback while `TRUST_PROXY` is enabled lets any client forge
  `X-Forwarded-For` and bypass login rate limiting; the server warns at startup
  if you do this, but it will not stop you.

## Reporting

Personal project on personal infrastructure. Report issues directly rather than
expecting a process.
