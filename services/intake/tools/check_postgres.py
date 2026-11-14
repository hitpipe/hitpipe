#!/usr/bin/env python3
"""Run the pinned migration against a disposable, network-isolated PostgreSQL.

Only synthetic data, a new container, tmpfs storage, and local Unix sockets.
No connection string or existing database is accepted. Python stdlib only.
"""
import hashlib
import json
import pathlib
import subprocess
import time
import uuid
from concurrent.futures import ThreadPoolExecutor

ROOT = pathlib.Path(__file__).resolve().parents[1]
IMAGE = 'docker.io/library/postgres@sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73'
CONTAINER = 'hit71-' + uuid.uuid4().hex[:12]
WS = '00000000-0000-4000-8000-000000000001'
OTHER = '00000000-0000-4000-8000-000000000002'
CONN = '00000000-0000-4000-8000-000000000011'
CONN2 = '00000000-0000-4000-8000-000000000012'
SIG = '00000000-0000-4000-8000-000000000021'
TENANT = ['source_connections', 'connection_secrets', 'ingress_quota', 'signals',
          'source_facts', 'gap_markers', 'inbox', 'outbox']
ROLES = ['hp_intake_app', 'hp_intake_worker', 'hp_intake_purge']
COUNT = 0


def command(args, **kwargs):
    return subprocess.run(args, text=True, capture_output=True, timeout=90, **kwargs)


def sql(role, query, database='intake_test'):
    return command(['docker', 'exec', '-i', CONTAINER, 'psql', '-X', '-qAt',
                    '-v', 'ON_ERROR_STOP=1', '-v', 'VERBOSITY=verbose',
                    '-U', role, '-d', database], input=query)


def require(result):
    if result.returncode:
        raise AssertionError(result.stderr + result.stdout)
    return result.stdout.strip()


def check(name, query, expected=None, role='hp_intake_app', error=None, scope=WS):
    global COUNT
    setup = 'BEGIN; SET LOCAL search_path=hp_intake,pg_catalog;'
    if scope is not None:
        setup += f"SET LOCAL hp.workspace_id='{scope}';"
    result = sql(role, setup + query + ';ROLLBACK;')
    if error:
        assert result.returncode and f'ERROR:  {error}:' in result.stderr, (name, result)
    else:
        output = require(result)
        if expected is not None:
            assert output == str(expected), (name, output, expected)
    COUNT += 1
    print(f'PASS {COUNT:03d} {name}', flush=True)


def signal(signal_id=SIG, event='evt_1', connection=CONN, size=1, state='HELD'):
    return f"""INSERT INTO signals(workspace_id,signal_id,connection_id,event_id,
      event_type,session_id,account_id,livemode,source_created_at,first_received_at,
      expires_at,serialized_bytes,state,reason_code) VALUES
      ('{WS}','{signal_id}','{connection}','{event}','checkout.session.completed',
       'cs_test','acct_test',false,'2026-11-01Z','2026-11-01Z','2026-11-04Z',
       {size},'{state}','TEST')"""


def fixtures():
    statements = ['SET search_path=hp_intake,pg_catalog;']
    for ws in [WS, OTHER]:
        statements += [f"""
        INSERT INTO source_connections VALUES ('{ws}','{CONN}','acct_test',false,1,'READY',NULL,NULL);
        INSERT INTO connection_secrets VALUES ('{ws}','{SIG}','{CONN}','{SIG}',
          'sha256:'||repeat('a',64),'test-key','WEBHOOK_SIGNING_SECRET',now(),NULL);
        INSERT INTO ingress_quota(workspace_id) VALUES ('{ws}');
        INSERT INTO source_facts VALUES ('{ws}','{SIG}','{SIG}','{CONN}','{SIG}',1,
          'sha256:'||repeat('a',64),true,now(),now(),now(),now()+interval '1 day');
        INSERT INTO gap_markers VALUES ('{ws}','{SIG}',now(),NULL,'TEST',1,now()+interval '1 day');
        INSERT INTO inbox VALUES ('{ws}','{SIG}','sha256:'||repeat('a',64),'HELD',NULL,now());
        INSERT INTO outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,expires_at)
          VALUES ('{ws}','{SIG}','TEST','{SIG}','sha256:'||repeat('a',64),1,
          'sha256:'||repeat('a',64),now()+interval '1 day');
        """]
    # Signals use their own transaction in each test; test RLS with one row per tenant.
    statements += [signal(state='APPLIED') + ';', signal(state='APPLIED').replace(WS, OTHER).replace("'evt_1'", "'evt_other'") + ';']
    require(sql('postgres', '\n'.join(statements)))


def catalog_tests():
    check('migration version', 'SELECT version FROM schema_version', 1)
    check('all ten tables owned by migrator', """SELECT count(*) FROM pg_tables
      WHERE schemaname='hp_intake' AND tableowner='hp_intake_migrator'""", 10)
    check('eight forced RLS tables and policies', """SELECT count(*) FROM pg_class c
      JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='hp_intake'
      AND c.relrowsecurity AND c.relforcerowsecurity AND EXISTS
      (SELECT 1 FROM pg_policy p WHERE p.polrelid=c.oid)""", 8)
    check('two trigger functions owned by migrator', """SELECT count(*) FROM pg_proc p
      JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='hp_intake'
      AND pg_get_userbyid(p.proowner)='hp_intake_migrator' AND NOT p.prosecdef""", 2)
    check('two enabled triggers', """SELECT count(*) FROM pg_trigger t JOIN pg_class c
      ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='hp_intake' AND NOT t.tgisinternal AND t.tgenabled='O'""", 2)
    check('all roles constrained', """SELECT count(*) FROM pg_roles WHERE rolname IN
      ('hp_intake_migrator','hp_intake_app','hp_intake_worker','hp_intake_purge')
      AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolbypassrls""", 4)
    check('three named partial indexes', """SELECT count(*) FROM pg_indexes WHERE
      schemaname='hp_intake' AND indexname IN ('signals_expiry','outbox_due','job_index_due')""", 3)
    # Check the complete table grant matrix, including the special worker-only job index.
    for role in ROLES:
        for table in TENANT + ['schema_version', 'job_index']:
            if table == 'schema_version':
                allowed = {'SELECT'}
            elif table == 'job_index':
                allowed = set() if role.endswith('_app') else {'SELECT','INSERT','UPDATE','DELETE'}
            elif table == 'source_facts':
                allowed = {'SELECT','DELETE'} if role.endswith('_purge') else {'SELECT','INSERT'}
            else:
                allowed = {'SELECT','INSERT','UPDATE'}
                if role.endswith('_purge'):
                    allowed.add('DELETE')
            for priv in ['SELECT','INSERT','UPDATE','DELETE','TRUNCATE','REFERENCES','TRIGGER']:
                check(f'grant {role} {table} {priv}',
                      f"SELECT has_table_privilege(current_user,'hp_intake.{table}','{priv}')",
                      't' if priv in allowed else 'f', role=role)
        for fn in ['maintain_signal_quota()', 'immutable_receipt()']:
            check(f'no direct function execution {role} {fn}',
                  f"SELECT has_function_privilege(current_user,'hp_intake.{fn}','EXECUTE')", 'f', role=role)
        for target in ['hp_intake_migrator'] + [r for r in ROLES if r != role]:
            check(f'no role escalation {role} -> {target}', f'SET ROLE {target}', role=role, error='42501')
        check(f'no schema creation {role}', 'CREATE TABLE hp_intake.forbidden(id integer)', role=role, error='42501')
        check(f'no DDL {role}', 'ALTER TABLE hp_intake.signals DISABLE ROW LEVEL SECURITY', role=role, error='42501')
        check(f'no version write {role}', 'DELETE FROM schema_version', role=role, error='42501')
    check('owner also subject to forced RLS', 'SELECT count(*) FROM source_connections', 0,
          role='hp_intake_migrator', scope=None)


def isolation_tests():
    for role in ROLES:
        check(f'actual login {role}', 'SELECT session_user=current_user', 't', role=role)
        for table in TENANT:
            check(f'no scope {role} {table}', f'SELECT count(*) FROM {table}', 0, role=role, scope=None)
            check(f'own scope {role} {table}', f'SELECT count(*) FROM {table}', 1, role=role)
            check(f'other rows hidden {role} {table}',
                  f"SELECT count(*) FROM {table} WHERE workspace_id='{OTHER}'", 0, role=role)
        check(f'foreign insert rejected {role}',
              f"INSERT INTO gap_markers VALUES ('{OTHER}','{CONN2}',now(),NULL,'TEST',0,now())",
              role=role, error='42501')
        check(f'foreign update changes zero rows {role}',
              f"WITH changed AS (UPDATE gap_markers SET known_count=10 WHERE workspace_id='{OTHER}' RETURNING 1) SELECT count(*) FROM changed",
              0, role=role)
        check(f'own update works {role}',
              'WITH changed AS (UPDATE gap_markers SET known_count=10 RETURNING 1) SELECT count(*) FROM changed',
              1, role=role)
    check('purge can delete own markers',
          'WITH gone AS (DELETE FROM gap_markers RETURNING 1) SELECT count(*) FROM gone', 1, role='hp_intake_purge')
    for role in ['hp_intake_app','hp_intake_worker']:
        for table in TENANT:
            check(f'delete denied {role} {table}', f'DELETE FROM {table}', role=role, error='42501')
        check(f'facts update denied {role}', 'UPDATE source_facts SET complete=false', role=role, error='42501')
    check('purge facts insert denied', 'INSERT INTO source_facts SELECT * FROM source_facts',
          role='hp_intake_purge', error='42501')
    check('purge facts update denied', 'UPDATE source_facts SET complete=false',
          role='hp_intake_purge', error='42501')
    check('app cannot read global job index', 'SELECT * FROM job_index', error='42501')
    for role in ['hp_intake_worker','hp_intake_purge']:
        check(f'global metadata index {role}',
              f"INSERT INTO job_index(job_id,workspace_id,kind,due_at) VALUES ('{SIG}','{OTHER}','TEST',now());SELECT count(*) FROM job_index",
              1, role=role, scope=None)
    check('scope resets after commit',
          'COMMIT; BEGIN; SET LOCAL search_path=hp_intake,pg_catalog; SELECT count(*) FROM signals', 0)


def receipt_tests():
    new_id = '00000000-0000-4000-8000-000000000022'
    insert = signal(new_id, 'evt_new')
    check('insert updates quota', insert + ';SELECT signal_count||\':\'||serialized_bytes FROM ingress_quota', '1:1')
    check('duplicate per connection rejected', signal(new_id), error='23505')
    check('same event allowed on different connection',
          f"INSERT INTO source_connections VALUES ('{WS}','{CONN2}','acct_other',false,1,'READY',NULL,NULL);" +
          signal(new_id, connection=CONN2) + ';SELECT count(*) FROM signals', 2)
    for column, value in [('workspace_id', OTHER),('signal_id',new_id),('connection_id',CONN2),
                          ('event_id','evt_changed'),('session_id','cs_changed'),
                          ('first_received_at','2026-11-02Z'),('expires_at','2026-11-05Z')]:
        check('immutable '+column, f"UPDATE signals SET {column}='{value}' WHERE signal_id='{SIG}'", error='P0001')
    check('72 hour boundary accepted', insert)
    check('72 hour overflow rejected', insert.replace("'2026-11-04Z'", "'2026-11-04T00:00:00.001Z'"), error='23514')
    for size in [0,16777217]:
        check(f'individual size {size} rejected', signal(new_id, 'evt_new', size=size), error='23514')
    check('16 MiB exact boundary', signal(new_id, 'evt_new', size=16777216) +
          ';SELECT serialized_bytes FROM ingress_quota', 16777216)
    check('aggregate bytes overflow', signal(new_id,'evt_new',size=16777216) + ';' +
          signal('00000000-0000-4000-8000-000000000023','evt_extra'), error='23514')
    bulk = f"""INSERT INTO signals SELECT workspace_id,md5('test-'||n)::uuid,connection_id,
      'evt_bulk_'||n,event_type,session_id,account_id,livemode,source_created_at,
      first_received_at,expires_at,1,'HELD',case_id,reason_code FROM signals
      CROSS JOIN generate_series(1,1000) n WHERE signal_id='{SIG}'"""
    check('1000 held signals accepted', bulk + ';SELECT signal_count FROM ingress_quota', 1000)
    check('1001st signal rejected', bulk + ';' + insert, error='23514')
    check('state release subtracts quota', insert +
          f";UPDATE signals SET state='APPLIED' WHERE signal_id='{new_id}';SELECT signal_count||':'||serialized_bytes FROM ingress_quota", '0:0')
    check('retained size update adjusts quota', insert +
          f";UPDATE signals SET serialized_bytes=5 WHERE signal_id='{new_id}';SELECT signal_count||':'||serialized_bytes FROM ingress_quota", '1:5')
    check('purge delete subtracts quota', insert +
          f";DELETE FROM signals WHERE signal_id='{new_id}';SELECT signal_count||':'||serialized_bytes FROM ingress_quota", '0:0', role='hp_intake_purge')
    check('applied signals do not use held quota', signal(new_id,'evt_new',state='APPLIED') +
          ';SELECT signal_count FROM ingress_quota', 0)
    check('no test writes persisted', 'SELECT count(*) FROM signals', 1)


def concurrency_tests():
    global COUNT
    require(sql('postgres', f"""SET search_path=hp_intake,pg_catalog;
      INSERT INTO signals SELECT workspace_id,md5('race-seed-'||n)::uuid,connection_id,
      'evt_seed_'||n,event_type,session_id,account_id,livemode,source_created_at,
      first_received_at,expires_at,1,'HELD',case_id,reason_code FROM signals
      CROSS JOIN generate_series(1,999) n WHERE workspace_id='{WS}' AND signal_id='{SIG}';"""))

    def race(same_event, expected_error):
        def contender(n):
            query = signal(f'00000000-0000-4000-8000-00000000003{n}',
                           'evt_race' if same_event else f'evt_race_{n}')
            return sql(['hp_intake_app','hp_intake_worker'][n],
                       f"BEGIN;SET LOCAL search_path=hp_intake,pg_catalog;SET LOCAL hp.workspace_id='{WS}';"
                       + query + ';SELECT pg_sleep(0.3);COMMIT;')
        with ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(contender, [0,1]))
        assert sum(r.returncode == 0 for r in results) == 1, results
        assert sum(f'ERROR:  {expected_error}:' in r.stderr for r in results) == 1, results

    race(False, '23514')
    check('concurrent app/worker cannot exceed 1000', 'SELECT signal_count FROM ingress_quota', 1000)
    require(sql('postgres', f"SET search_path=hp_intake,pg_catalog;DELETE FROM signals WHERE workspace_id='{WS}' AND state='HELD';"))
    race(True, '23505')
    check('concurrent duplicate consumes one slot', 'SELECT signal_count FROM ingress_quota', 1)
    require(sql('postgres', f"SET search_path=hp_intake,pg_catalog;DELETE FROM signals WHERE workspace_id='{WS}' AND state='HELD';"))
    check('cleanup leaves zero held quota', 'SELECT signal_count FROM ingress_quota', 0)


def main():
    migration = ROOT / 'migrations/0001_intake.sql'
    print(json.dumps({'image': IMAGE, 'migration_sha256': hashlib.sha256(migration.read_bytes()).hexdigest(),
                      'network': 'none', 'storage': 'tmpfs', 'host_ports': []}), flush=True)
    try:
        require(command(['docker','run','-d','--name',CONTAINER,'--network','none',
                         '--tmpfs','/var/lib/postgresql:rw,noexec,nosuid,size=256m',
                         '-e','POSTGRES_HOST_AUTH_METHOD=trust', IMAGE]))
        for _ in range(60):
            if sql('postgres', 'SELECT 1', 'postgres').returncode == 0:
                break
            time.sleep(0.5)
        else:
            raise AssertionError('test database did not start')
        version = require(sql('postgres','SHOW server_version', 'postgres'))
        print('server_version=' + version, flush=True)
        assert version == '18.7', version
        require(sql('postgres', (ROOT/'tests/postgres/bootstrap.sql').read_text(), 'postgres'))
        require(sql('hp_intake_migrator', migration.read_text()))
        print('migration applied by hp_intake_migrator', flush=True)
        fixtures()
        catalog_tests()
        isolation_tests()
        receipt_tests()
        concurrency_tests()
        print(f'PASS: {COUNT} isolated PostgreSQL checks; no webhook, file durability, user authorization or deployment claim.')
    finally:
        require(command(['docker','rm','-f',CONTAINER]))


if __name__ == '__main__':
    main()
