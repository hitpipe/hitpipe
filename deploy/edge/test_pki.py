"""Make short-lived, offline-only certificate material for the isolated edge lab."""

import os
import secrets
import shutil
import stat
import subprocess
import tempfile
from pathlib import Path


def _openssl(step: str, *args: str | Path) -> None:
    try:
        subprocess.run(
            ["openssl", *map(str, args)],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=True,
            timeout=15,
        )
    except (OSError, subprocess.SubprocessError):
        raise RuntimeError(f"OpenSSL failed during synthetic {step}") from None


def _text(path: Path, content: str) -> None:
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w", encoding="ascii") as f:
        f.write(content)


def _copy(source: Path, dest: Path) -> None:
    with source.open("rb") as inp, os.fdopen(
        os.open(dest, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb"
    ) as out:
        shutil.copyfileobj(inp, out)


def _key(path: Path) -> None:
    _text(path, "")
    _openssl("key generation", "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256", "-out", path)
    path.chmod(0o600)


def _serial() -> str:
    # X.509 serials must be positive and at most 20 octets, including sign.
    return "0x" + format(secrets.randbits(127) | (1 << 127), "x")


def _ca(working: Path, name: str, config: Path) -> tuple[Path, Path]:
    key, cert = working / f"{name}.key", working / f"{name}.crt"
    _key(key)
    _openssl(
        "CA issuance",
        "req", "-new", "-x509", "-sha256", "-days", "2", "-key", key,
        "-subj", f"/CN=HIT-141 synthetic {name} CA", "-config", config,
        "-extensions", "fixture_ca", "-set_serial", _serial(), "-out", cert,
    )
    cert.chmod(0o600)
    return cert, key


def _leaf(working: Path, config: Path, deploy: Path, name: str, dns: str, eku: str, issuer: tuple[Path, Path]) -> None:
    key, cert = deploy / f"{name}.key", deploy / f"{name}.crt"
    csr = working / "leaf.csr"
    extensions = working / "leaf.ext"
    _key(key)
    _text(
        extensions,
        "[fixture_leaf]\n"
        "basicConstraints = critical, CA:FALSE\n"
        "keyUsage = critical, digitalSignature\n"
        f"extendedKeyUsage = {eku}\n"
        f"subjectAltName = DNS:{dns}\n"
        "subjectKeyIdentifier = hash\n"
        "authorityKeyIdentifier = keyid:always\n",
    )
    _openssl("leaf request", "req", "-new", "-sha256", "-key", key, "-subj", f"/CN={dns}", "-config", config, "-out", csr)
    _openssl(
        "leaf issuance",
        "x509", "-req", "-sha256", "-days", "1", "-in", csr,
        "-CA", issuer[0], "-CAkey", issuer[1], "-set_serial", _serial(),
        "-extfile", extensions, "-extensions", "fixture_leaf", "-out", cert,
    )
    cert.chmod(0o600)
    csr.unlink()
    extensions.unlink()


def generate(root: Path) -> None:
    """Write only per-role material in an empty, owned 0700 directory.

    The three CA private keys and request/config files are removed before
    returning. The caller owns cleanup of the remaining disposable fixture.
    """
    attrs = root.stat(follow_symlinks=False)
    if (
        not stat.S_ISDIR(attrs.st_mode)
        or attrs.st_uid != os.getuid()
        or stat.S_IMODE(attrs.st_mode) != 0o700
        or any(root.iterdir())
    ):
        raise ValueError("synthetic PKI output must be an empty, owned 0700 directory")
    with tempfile.TemporaryDirectory(prefix=".issuance-", dir=root) as scratch:
        working = Path(scratch)
        config = working / "req.cnf"
        _text(
            config,
            "[req]\n"
            "distinguished_name = fixture_dn\n"
            "prompt = no\n"
            "[fixture_dn]\n"
            "[fixture_ca]\n"
            "basicConstraints = critical, CA:TRUE, pathlen:0\n"
            "keyUsage = critical, keyCertSign, cRLSign\n"
            "subjectKeyIdentifier = hash\n",
        )
        public = _ca(working, "public", config)
        service = _ca(working, "service", config)
        foreign = _ca(working, "foreign", config)
        edge, client = root / "edge", root / "client"
        edge.mkdir(mode=0o700)
        client.mkdir(mode=0o700)
        _leaf(working, config, edge, "public", "app.edge.test", "serverAuth", public)
        _leaf(working, config, edge, "idp", "idp.edge.test", "serverAuth", public)
        _leaf(working, config, edge, "edge", "edge", "clientAuth", service)
        _copy(public[0], client / "public-ca.crt")
        _copy(service[0], edge / "service-ca.crt")

        for name, dns, issuer in (
            ("iam", "iam", service),
            ("bff", "bff", service),
            ("intake", "intake", service),
            ("keycloak", "keycloak", service),
            ("iam-wrong-san", "different-iam", service),
            ("iam-untrusted-ca", "iam", foreign),
        ):
            role = root / name
            role.mkdir(mode=0o700)
            _leaf(working, config, role, "server", dns, "serverAuth", issuer)
            # The negative server certificates never change whom the fixture
            # accepts as its client. Only Caddy's server verification changes.
            _copy(service[0], role / "service-ca.crt")
