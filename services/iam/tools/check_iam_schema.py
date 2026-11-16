#!/usr/bin/env python3
"""Проверить миграцию и защиту IAM на одноразовой PostgreSQL; внешняя БД не используется."""
import sys

from check_postgres_session_lookup import main


if __name__ == "__main__":
    sys.exit(main(task="HIT-81", schema_checks=True))
