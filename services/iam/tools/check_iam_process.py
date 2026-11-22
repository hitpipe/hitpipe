#!/usr/bin/env python3
"""Qualify the IAM process and its regression suite with disposable PostgreSQL."""

import sys

from check_postgres_session_lookup import main


if __name__ == "__main__":
    sys.exit(main(task="HIT-138", schema_checks=True, authorization_checks=True, issuance_checks=True))
