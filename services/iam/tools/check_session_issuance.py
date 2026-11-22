#!/usr/bin/env python3
"""Проверить создание сессий IAM на одноразовой PostgreSQL 18.7, без внешнего DSN."""

import sys

from check_postgres_session_lookup import main


if __name__ == "__main__":
    sys.exit(main(task="HIT-83", schema_checks=True, issuance_checks=True))
