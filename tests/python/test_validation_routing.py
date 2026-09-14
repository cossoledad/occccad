import unittest

import tasks


class ValidationRoutingTest(unittest.TestCase):
    def scopes(self, *paths: str) -> set[str]:
        scopes, _ = tasks._validation_scopes_for_paths(list(paths))
        return scopes

    def test_documentation_only_needs_no_executable_validation(self):
        self.assertEqual(self.scopes("docs/README.md", "kernel/assembly/AGENTS.md"), set())

    def test_local_domains_remain_scoped(self):
        self.assertEqual(self.scopes("kernel/assembly/src/solver.cpp"), {"assembly"})
        self.assertEqual(self.scopes("workers/geometry/sketch/src/solver.cpp"), {"sketch"})
        self.assertEqual(self.scopes("kernel/occt/src/occt_kernel.cpp"), {"geometry"})
        self.assertEqual(self.scopes("services/internal/workspace/service.go"), {"workspace"})
        self.assertEqual(self.scopes("web/apps/cad/src/app/app.tsx"), {"web"})

    def test_independent_domains_are_combined(self):
        self.assertEqual(
            self.scopes("kernel/assembly/src/solver.cpp", "web/apps/cad/src/app/app.tsx"),
            {"assembly", "web"},
        )

    def test_shared_contracts_and_unknown_paths_escalate_to_all(self):
        for path in (
            "proto/occccad/worker/v1/geometry_worker.proto",
            "services/internal/database/migrations/0016_example.sql",
            "workers/geometry/src/main.cpp",
            "tasks.py",
            "tests/python/test_validation_routing.py",
            "unexpected.file",
        ):
            with self.subTest(path=path):
                self.assertEqual(self.scopes(path), {"all"})

    def test_match_keeps_specific_checks_inside_one_domain(self):
        assembly = tasks._check_steps({"assembly"}, "Debug", "DirectedAngle")
        self.assertEqual([name for name, _, _ in assembly], ["Assembly C++ build", "Assembly CTest"])
        self.assertIn("DirectedAngle", assembly[1][1])

        web = tasks._check_steps({"web"}, "Debug", "realtime")
        self.assertEqual([name for name, _, _ in web], ["Web scenarios"])
        self.assertIn("realtime", web[0][1])

    def test_go_json_output_counts_runs_and_restores_diagnostics(self):
        output = "\n".join((
            '{"Action":"run","Package":"example","Test":"TestUndo"}',
            '{"Action":"output","Package":"example","Test":"TestUndo","Output":"failure detail\\n"}',
            '{"Action":"fail","Package":"example","Test":"TestUndo"}',
        ))
        self.assertEqual(tasks._go_test_events(output), (1, "failure detail"))


if __name__ == "__main__":
    unittest.main()
