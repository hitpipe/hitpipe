#!/usr/bin/env python3
"""Run the HIT-84 authorization checks against disposable local PostgreSQL."""

import sys

from check_postgres_session_lookup import main


if __name__ == "__main__":
    sys.exit(main(task="HIT-84", schema_checks=True, authorization_checks=True))
