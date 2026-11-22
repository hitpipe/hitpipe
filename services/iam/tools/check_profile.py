#!/usr/bin/env python3
"""Run HIT-85 HTTPS, PostgreSQL and owner-content checks in a disposable sandbox."""

import sys

from check_postgres_session_lookup import main


if __name__ == "__main__":
    sys.exit(main(task="HIT-85", schema_checks=True, authorization_checks=True))
