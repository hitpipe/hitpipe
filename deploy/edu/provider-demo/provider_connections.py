"""Official provider API calls for onboarding a workspace on the server."""
from concurrent.futures import ThreadPoolExecutor
import json
import re
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import ProxyHandler, Request, build_opener

from provider_egress import CLICKUP, STRIPE, VERSION, NoRedirect

STRIPE_KEY = re.compile(r"(?:rk|sk)_test_[a-zA-Z0-9]{16,220}\Z")
CLICKUP_KEY = re.compile(r"pk_[a-zA-Z0-9_-]{10,240}\Z")
STRIPE_ACCOUNT = re.compile(r"acct_[a-zA-Z0-9]{8,64}\Z")
STRIPE_PRICE = re.compile(r"price_[a-zA-Z0-9]{8,64}\Z")
STRIPE_PRODUCT = re.compile(r"[a-zA-Z0-9_-]{2,100}\Z")
NUMBER = re.compile(r"[1-9][0-9]{0,19}\Z")
WEBHOOK_SECRET = re.compile(r"whsec_[a-zA-Z0-9_-]{16,240}\Z")
WEBHOOK_ENDPOINT = re.compile(r"we_[a-zA-Z0-9]{8,80}\Z")
EVENTS = ("checkout.session.completed", "checkout.session.async_payment_succeeded")
MAX_CATALOG = 100


class ProviderError(Exception):
    def __init__(self, message, *, status=422):
        super().__init__(message)
        self.status = status


def call(provider, path, token, form=None, idempotency=None):
    host, label = (STRIPE, "Stripe") if provider == "stripe" else (CLICKUP, "ClickUp")
    headers = {"Authorization": f"Bearer {token}" if provider == "stripe" else token,
               "Accept": "application/json"}
    if provider == "stripe":
        headers["Stripe-Version"] = VERSION
    if form is not None:
        headers["Content-Type"] = "application/x-www-form-urlencoded"
    if idempotency is not None:
        headers["Idempotency-Key"] = idempotency
    req = Request(host + path, data=urlencode(form, doseq=True).encode() if form is not None else None,
                  headers=headers, method="POST" if form is not None else "GET")
    try:
        with build_opener(ProxyHandler({}), NoRedirect).open(req, timeout=6) as res:
            payload = res.read(2_000_001)
    except HTTPError as exc:
        if exc.code in {401, 403}:
            raise ProviderError(f"{label} отклонил ключ или доступ к ресурсу. Проверьте права ключа.") from None
        if exc.code in {404, 400}:
            detail = "Проверьте выбранный объект и его настройки."
            if form is not None:
                detail = "Проверьте разрешение на создание вебхука Stripe Sandbox."
            raise ProviderError(f"{label} отклонил настройку. {detail}") from None
        raise ProviderError(f"{label} временно недоступен. Повторите подключение.", status=503) from None
    except (URLError, TimeoutError):
        raise ProviderError(f"{label} не ответил. Повторите подключение.", status=503) from None
    try:
        doc = json.loads(payload) if len(payload) <= 2_000_000 else None
    except (UnicodeDecodeError, ValueError):
        doc = None
    if not isinstance(doc, dict):
        raise ProviderError(f"{label} вернул неожиданный ответ.", status=502)
    return doc


def identifier(raw, pattern=NUMBER):
    value = str(raw) if type(raw) in {str, int} else ""
    if not pattern.fullmatch(value):
        raise ProviderError("Сервис вернул неподдерживаемый идентификатор.", status=502)
    return value


def display(raw, fallback):
    name = " ".join(raw.split()) if isinstance(raw, str) else ""
    return (name or fallback)[:120]


def members(doc):
    raw = doc.get("members")
    if not isinstance(raw, list) or len(raw) > MAX_CATALOG:
        raise ProviderError("ClickUp вернул слишком большой или недоступный список исполнителей.")
    result = []
    for user in raw:
        member = user.get("user", user)
        result.append({"id": identifier(member.get("id")), "name": display(member.get("username"), "Исполнитель")})
    return result


class Connections:
    def __init__(self, stripe, clickup, transport=call):
        if not isinstance(stripe, str) or not STRIPE_KEY.fullmatch(stripe):
            raise ProviderError("Нужен секретный ключ Stripe Sandbox с префиксом sk_test_ или rk_test_.")
        if not isinstance(clickup, str) or not CLICKUP_KEY.fullmatch(clickup):
            raise ProviderError("Нужен персональный API-токен ClickUp с префиксом pk_.")
        self.stripe_key = stripe
        self.clickup_key = clickup
        self.transport = transport

    def stripe(self, path, form=None, idempotency=None):
        return self.transport("stripe", path, self.stripe_key, form, idempotency)

    def clickup(self, path):
        return self.transport("clickup", path, self.clickup_key)

    def stripe_account(self):
        return identifier(self.stripe("/v1/account").get("id"), STRIPE_ACCOUNT)

    def price(self, item):
        price_id = identifier(item.get("id"), STRIPE_PRICE)
        if item.get("livemode") is not False or item.get("active") is not True or item.get("type") != "one_time":
            raise ProviderError("Выберите активный разовый тариф в Stripe Sandbox.")
        product = item.get("product")
        if isinstance(product, str):
            product_id = identifier(product, STRIPE_PRODUCT)
            product = self.stripe(f"/v1/products/{product_id}")
        currency = item.get("currency", "")
        amount = item.get("unit_amount")
        if (not isinstance(product, dict) or product.get("active") is not True
                or product.get("livemode") is not False or re.fullmatch("[a-z]{3}", currency) is None
                or amount is not None and (type(amount) is not int or amount < 0)):
            raise ProviderError("Тариф Stripe и его услуга недоступны в Sandbox.")
        return {"id": price_id, "label": display(item.get("nickname"), display(product.get("name"), "Разовый тариф")),
                "currency": currency, "unitAmount": amount}

    def stripe_catalog(self):
        account = self.stripe_account()
        prices = []
        cursor = None
        while True:
            query = {"active": "true", "type": "one_time", "limit": 100, "expand[]": ["data.product"]}
            if cursor:
                query["starting_after"] = cursor
            page = self.stripe("/v1/prices?" + urlencode(query, doseq=True))
            data = page.get("data")
            if not isinstance(data, list) or len(prices) + len(data) > MAX_CATALOG:
                raise ProviderError("В Sandbox слишком много активных тарифов для экрана подключения.")
            prices.extend(self.price(item) for item in data)
            if page.get("has_more") is not True:
                return {"account": account, "prices": prices}
            next_cursor = prices[-1]["id"] if data else None
            if next_cursor is None or next_cursor == cursor:
                raise ProviderError("Невозможно полностью прочитать каталог Stripe.", status=502)
            cursor = next_cursor

    def spaces(self, team):
        return self.clickup(f"/api/v2/team/{team}/space?archived=false").get("spaces", [])

    def lists(self, space):
        base = f"/api/v2/space/{space}"
        direct = self.clickup(base + "/list?archived=false").get("lists", [])
        folders = self.clickup(base + "/folder?archived=false").get("folders", [])
        if len(folders) > MAX_CATALOG:
            raise ProviderError("В пространстве ClickUp слишком много папок для экрана подключения.")
        nested = []
        for folder in folders:
            folder_id = identifier(folder.get("id"))
            nested.extend(self.clickup(f"/api/v2/folder/{folder_id}/list?archived=false").get("lists", []))
        return direct + nested

    def clickup_user(self):
        user = self.clickup("/api/v2/user")["user"]
        return {"id": identifier(user.get("id")), "name": display(user.get("username"), "Текущий пользователь")}

    def clickup_assignees(self, list_id, current):
        direct = members(self.clickup(f"/api/v2/list/{list_id}/member"))
        return direct if any(candidate["id"] == current["id"] for candidate in direct) else [current, *direct]

    def clickup_catalog(self):
        current = self.clickup_user()
        teams = self.clickup("/api/v2/team").get("teams", [])
        if len(teams) > MAX_CATALOG:
            raise ProviderError("Слишком много рабочих областей ClickUp для экрана подключения.")
        workspaces = []
        count = 0
        for team in teams:
            workspace = identifier(team.get("id"))
            lists = []
            for space in self.spaces(workspace):
                for item in self.lists(identifier(space.get("id"))):
                    count += 1
                    if count > MAX_CATALOG:
                        raise ProviderError("Слишком много доступных списков ClickUp для экрана подключения.")
                    list_id = identifier(item.get("id"))
                    assignees = self.clickup_assignees(list_id, current)
                    lists.append({"id": list_id, "name": display(item.get("name"), "Список"), "assignees": assignees})
            workspaces.append({"id": workspace, "name": display(team.get("name"), "Рабочая область"), "lists": lists})
        return {"workspaces": workspaces}

    def inspect(self):
        with ThreadPoolExecutor(max_workers=2) as pool:
            stripe = pool.submit(self.stripe_catalog)
            clickup = pool.submit(self.clickup_catalog)
            return {"state": "VERIFIED", "stripe": stripe.result(), "clickup": clickup.result()}

    def validate_selection(self, data):
        price = identifier(data.get("sourcePrice"), STRIPE_PRICE)
        workspace = identifier(data.get("destinationWorkspace"))
        target = identifier(data.get("targetList"))
        selected = identifier(data.get("assignee"))
        account = self.stripe_account()
        price_doc = self.price(self.stripe(f"/v1/prices/{price}?expand[]=product"))
        if price_doc["id"] != price:
            raise ProviderError("Выбранный тариф отличается от полученного от Stripe.")
        raw = self.clickup(f"/api/v2/list/{target}")
        recorded_space = str(raw.get("space", {}).get("id"))
        if (str(raw.get("id")) != target or raw.get("archived") is True
                or recorded_space not in {str(space.get("id")) for space in self.spaces(workspace)}
                or selected not in {user["id"] for user in self.clickup_assignees(target, self.clickup_user())}):
            raise ProviderError("Выберите доступные список и исполнителя в одной рабочей области ClickUp.")
        return {"sourceAccount": account, "sourcePrice": price, "destinationWorkspace": workspace,
                "targetList": target, "assignee": selected, "label": price_doc["label"]}

    def webhook(self, url, idempotency):
        doc = self.stripe("/v1/webhook_endpoints", {
            "url": url, "connect": "false", "api_version": VERSION, "enabled_events[]": EVENTS,
        }, idempotency)
        if (doc.get("url") != url or doc.get("livemode") is not False or doc.get("api_version") != VERSION
                or sorted(doc.get("enabled_events", [])) != sorted(EVENTS) or doc.get("status") != "enabled"
                or not isinstance(doc.get("secret"), str) or not WEBHOOK_SECRET.fullmatch(doc["secret"])):
            raise ProviderError("Stripe не подтвердил webhook Sandbox. Повторите подключение.", status=503)
        return identifier(doc.get("id"), WEBHOOK_ENDPOINT), doc["secret"]
