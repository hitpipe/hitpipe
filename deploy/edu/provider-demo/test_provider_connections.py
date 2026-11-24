"""Provider discovery selects the authenticated owner even with inherited access."""
import unittest

from provider_connections import Connections, ProviderError, VERSION


class RealProviderContractTests(unittest.TestCase):
    def setUp(self):
        self.calls = []
        self.api = Connections("sk_test_" + "s" * 24, "pk_" + "c" * 24, self.provider)

    def provider(self, provider, path, key, form=None, request_id=None):
        self.calls.append((provider, path, form, request_id))
        if provider == "stripe":
            if path == "/v1/account":
                return {"id": "acct_newOwner123"}
            if path.startswith("/v1/prices"):
                price = {"id": "price_active123", "unit_amount": 500, "currency": "usd", "livemode": False,
                         "type": "one_time", "active": True,
                         "product": {"id": "prod_new", "name": "Demo", "livemode": False, "active": True}}
                return {"data": [price], "has_more": False} if path.startswith("/v1/prices?") else price
            if path == "/v1/webhook_endpoints":
                self.assertEqual(form["api_version"], VERSION)
                return {"url": form["url"], "api_version": VERSION, "livemode": False, "status": "enabled",
                        "id": "we_newExample", "secret": "whsec_" + "h"*24, "enabled_events": form["enabled_events[]"]}
        clickup = {
            "/api/v2/user": {"user": {"id": 42, "username": "Owner"}},
            "/api/v2/team": {"teams": [{"id": 90, "name": "Team"}]},
            "/api/v2/team/90/space?archived=false": {"spaces": [{"id": "123"}]},
            "/api/v2/space/123/list?archived=false": {"lists": [{"id": "456", "name": "List"}]},
            "/api/v2/space/123/folder?archived=false": {"folders": []},
            "/api/v2/list/456": {"id": "456", "space": {"id": "123"}},
            "/api/v2/list/456/member": {"members": []},
        }
        return clickup[path]

    def test_owner_with_access_via_space_can_choose_self(self):
        data = self.api.inspect()
        self.assertEqual(data["stripe"]["prices"][0]["unitAmount"], 500)
        self.assertEqual(data["clickup"]["workspaces"][0]["lists"][0]["assignees"], [{"id": "42", "name": "Owner"}])
        choice = {"sourcePrice": "price_active123", "destinationWorkspace": "90",
                  "targetList": "456", "assignee": "42"}
        self.assertEqual(self.api.validate_selection(choice)["label"], "Demo")
        with self.assertRaises(ProviderError):
            self.api.validate_selection({**choice, "assignee": "43"})
        self.api.webhook("https://hitpipe.dkar-dev.ru/webhooks/stripe/fixture", "workspace-request-key")
        self.assertEqual(self.calls[-1][3], "workspace-request-key")


if __name__ == "__main__":
    unittest.main()
