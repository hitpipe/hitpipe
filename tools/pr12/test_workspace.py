"""Checks wrapper boundaries only; these are not student module tests."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock
import subprocess

SPEC = importlib.util.spec_from_file_location('pr12_workspace', Path(__file__).with_name('workspace.py'))
workspace = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(workspace)


class BootstrapBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.old_root, self.old_cfg = workspace.ROOT, workspace.CFG
        workspace.ROOT = Path(self.temp.name)
        workspace.CFG = {'kind':'backend','components':['iam','intake']}

    def tearDown(self):
        workspace.ROOT, workspace.CFG = self.old_root, self.old_cfg
        self.temp.cleanup()

    def test_current_project_os_pointers_do_not_require_retired_manual_route(self):
        entries = {
            'AGENTS.md': 'https://github.com/hitpipe/project-os/blob/main/AGENT_MASTER_PROMPT.md',
            'GEMINI.md': '@./AGENTS.md',
            '.hermes.md': 'Read AGENTS.md',
            '.agents/rules/project-os.md': 'Read AGENTS.md',
        }
        for relative, content in entries.items():
            path = workspace.ROOT / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
        workspace.validate_loaders()
        with (workspace.ROOT / 'AGENTS.md').open('a') as stream:
            stream.write('\n## Explicit manual laboratory route')
        with self.assertRaisesRegex(SystemExit, 'retired laboratory'):
            workspace.validate_loaders()
        (workspace.ROOT / 'AGENTS.md').write_text(entries['AGENTS.md'])
        (workspace.ROOT / '.hermes.md').unlink()
        with self.assertRaisesRegex(SystemExit, 'entry absent'):
            workspace.validate_loaders()

    def test_absent_module_reports_not_implemented_not_pass(self):
        output=io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertIsNone(workspace.implemented('iam'))
        result=json.loads(output.getvalue())
        self.assertEqual(result['status'],'NOT_IMPLEMENTED')
        self.assertEqual(result['student_checks'],'NOT_RUN')

    def test_require_rejects_missing_student_module(self):
        with self.assertRaisesRegex(SystemExit,'required'):
            workspace.implemented('iam',required=True)

    def test_partial_module_cannot_silently_skip(self):
        path=workspace.ROOT/'services/iam/main.go';path.parent.mkdir(parents=True);path.write_text('package main\n')
        with self.assertRaisesRegex(SystemExit,'manifest|lab-component'):
            workspace.implemented('iam')

    def test_domain_cannot_import_network_adapter(self):
        path=workspace.ROOT/'services/iam/internal/domain/session/policy.go';path.parent.mkdir(parents=True)
        path.write_text('package session\nimport "net/http"\n')
        with self.assertRaisesRegex(SystemExit,'forbidden domain import'):
            workspace.import_rules('iam')

    def test_import_guard_does_not_treat_ordinary_string_as_import(self):
        path=workspace.ROOT/'services/iam/internal/domain/session/policy.go';path.parent.mkdir(parents=True)
        path.write_text('package session\nconst example = "net/http"\n')
        workspace.import_rules('iam')

    def test_manifest_rejects_fixture_path_escape(self):
        folder=workspace.ROOT/'services/iam';folder.mkdir(parents=True)
        outside=workspace.ROOT/'secret.json';outside.write_text('{}')
        (folder/'lab-component.json').write_text(json.dumps({'schema_version':1,'component':'iam','positive_fixture':'../../secret.json','negative_fixture':'../../secret.json','positive_output_contains':['valid'],'expected_test_names':['TestA','TestB','TestC']}))
        with self.assertRaisesRegex(SystemExit,'inside component'):
            workspace.implemented('iam')


    def test_node_skipped_or_todo_named_tests_do_not_count_as_pass(self):
        result="ok 1 - normal # SKIP\nok 2 - boundary # TODO\nok 3 - invalid\n# tests 3\n# pass 1\n"
        with self.assertRaisesRegex(SystemExit,'actually pass'):
            workspace.verify_node_results(result,['normal','boundary','invalid'])

    def test_node_nested_actual_passes_are_accepted(self):
        result="    ok 1 - normal\n    ok 2 - boundary\n    ok 3 - invalid\n# tests 3\n# pass 3\n"
        workspace.verify_node_results(result,['normal','boundary','invalid'])

    def test_command_timeout_is_not_a_success(self):
        with mock.patch.object(workspace.subprocess,'run',side_effect=subprocess.TimeoutExpired(['tool'],1)):
            with self.assertRaisesRegex(SystemExit,'timeout'):
                workspace.execute(['tool'],timeout=1)

    def test_frontend_build_cleans_stale_output_and_sets_release_flags(self):
        workspace.CFG={'kind':'frontend'}
        root=workspace.ROOT
        tsc=root/'tooling/pr12/frontend/node_modules/typescript/bin/tsc'
        tsc.parent.mkdir(parents=True);tsc.write_text('not executed')
        stale=root/'.pr12-build/frontend/old.test.js'
        stale.parent.mkdir(parents=True);stale.write_text('must not survive')
        observed=[]
        def fake_execute(args,**kwargs):
            observed.append(list(map(str,args)))
            if args[:3]==['git','rev-parse','HEAD']: return '1234567890abcdef\n'
            if args[:3]==['git','status','--porcelain']: return ''
            self.assertFalse(stale.exists())
            (root/'.pr12-build/frontend').mkdir(parents=True,exist_ok=True)
            return ''
        with mock.patch.object(workspace,'execute',side_effect=fake_execute):
            workspace.frontend_build('release')
        compiler=[a for a in observed if a[0]=='node'][0]
        self.assertEqual(compiler[-4:],['--sourceMap','false','--removeComments','true'])
        metadata=(root/'tools/pr12/component/build-info.ts').read_text()
        self.assertIn('"buildMode": "release"',metadata)
        self.assertIn('1234567890abcdef',metadata)


    def test_domain_dot_import_cannot_bypass_guard(self):
        path=workspace.ROOT/'services/iam/internal/domain/policy.go';path.parent.mkdir(parents=True)
        path.write_text('package domain\nimport . "net/http"\n')
        with self.assertRaisesRegex(SystemExit,'dot import'):
            workspace.import_rules('iam')

    def test_domain_raw_import_cannot_bypass_guard(self):
        path=workspace.ROOT/'services/iam/internal/domain/policy.go';path.parent.mkdir(parents=True)
        path.write_text('package domain\nimport `net/http`\n')
        with self.assertRaisesRegex(SystemExit,'canonical double-quoted'):
            workspace.import_rules('iam')

    def test_escaped_import_path_is_rejected_not_silently_allowed(self):
        text='package domain\nimport "net' + chr(92) + 'x2fhttp"\n'
        with self.assertRaisesRegex(SystemExit,'canonical double-quoted'):
            workspace.go_imports(text)

    def test_same_line_import_still_checks_layer(self):
        path=workspace.ROOT/'services/iam/internal/domain/policy.go';path.parent.mkdir(parents=True)
        path.write_text('package domain; import "net/http"\n')
        with self.assertRaisesRegex(SystemExit,'forbidden domain import'):
            workspace.import_rules('iam')

    def test_import_reader_handles_comments_aliases_and_nonimport_strings(self):
        text = ( 'package domain\n/* import "net/http" */\n'
                 'import ( alias "fmt"; _ "errors" )\n'
                 'const text = `import "database/sql"`\n'
                 '// import "os"\n' )
        self.assertEqual(workspace.go_imports(text), ['fmt','errors'])

    def test_node_test_name_prefix_is_not_an_exact_pass(self):
        result='ok 1 - normal something_else\nok 2 - boundary\nok 3 - invalid\n# tests 3\n# pass 3\n'
        with self.assertRaisesRegex(SystemExit,'actually pass'):
            workspace.verify_node_results(result,['normal','boundary','invalid'])

    def test_node_exact_name_with_non_skip_tap_annotation_is_accepted(self):
        result='ok 1 - normal # duration hint\nok 2 - boundary\nok 3 - invalid\n# tests 3\n# pass 3\n'
        workspace.verify_node_results(result,['normal','boundary','invalid'])


if __name__=='__main__':
    unittest.main()
