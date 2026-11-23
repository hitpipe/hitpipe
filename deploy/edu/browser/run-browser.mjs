import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { isAbsolute, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const cookieName = '__Host-hp_session';
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;

function localUrl(value, field) {
  let url;
  try {
    url = new URL(value);
  } catch {
    throw new Error(`${field}: нужен абсолютный локальный URL`);
  }
  assert.ok(
    ['http:', 'https:'].includes(url.protocol)
      && ['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)
      && !url.username && !url.password && !url.hash,
    `${field}: разрешён только локальный адрес без учётных данных`,
  );
  return url;
}

function validateConfig(config) {
  config.origin = localUrl(config.origin, 'origin').origin;
  config.clickupProofUrl = localUrl(config.clickupProofUrl, 'clickupProofUrl');
  if (config.stripeTriggerUrl) config.stripeTriggerUrl = localUrl(config.stripeTriggerUrl, 'stripeTriggerUrl');
  for (const key of ['workspaceId', 'stripeConnectionId', 'clickupConnectionId']) {
    assert.ok(typeof config[key] === 'string' && uuid.test(config[key]), `${key}: нужен UUID`);
  }
  for (const key of ['priceAlias', 'listAlias', 'assigneeAlias', 'sessionToken', 'artifactDir']) {
    assert.ok(typeof config[key] === 'string' && config[key].length > 0, `${key}: требуется значение`);
  }
  assert.ok(isAbsolute(config.artifactDir), 'artifactDir: нужен абсолютный путь');
  assert.ok(config.schoolAi === undefined || typeof config.schoolAi === 'boolean', 'schoolAi: ожидается true или false');
  assert.ok(!/[\x00-\x20\x7f;,]/.test(config.sessionToken), 'Некорректная форма значения сеанса');
  return config;
}

function loadPlaywright(config) {
  const frontend = config.frontendRoot;
  assert.ok(typeof frontend === 'string' && isAbsolute(frontend), 'Для Playwright нужен абсолютный путь к собранному интерфейсу');
  const require = createRequire(join(frontend, 'package.json'));
  return require(config.playwrightModule ?? '@playwright/test');
}

async function proofOfDelivery(url) {
  const deadline = Date.now() + 35_000;
  while (Date.now() < deadline) {
    const response = await fetch(url, { signal: AbortSignal.timeout(2000), redirect: 'error' });
    if (response.status === 200) {
      const proof = await response.json();
      assert.equal(proof.create_count, 1, 'ClickUp должен получить ровно один Create Task');
      assert.ok(Number.isInteger(proof.read_count) && proof.read_count >= 1, 'ClickUp должен подтвердить задачу чтением');
      assert.ok(typeof proof.task_id === 'string' && /^[A-Za-z0-9_-]+$/.test(proof.task_id),
        'В доказательстве нужен ID задачи ClickUp');
      assert.ok(typeof proof.case_id === 'string' && uuid.test(proof.case_id),
        'В доказательстве нужен ID обработанного дела');
      assert.ok(typeof proof.title === 'string' && proof.title.length > 0,
        'В доказательстве нужно название созданной задачи');
      return proof;
    }
    assert.ok([202, 404, 503].includes(response.status), `Стенд доказательств вернул HTTP ${response.status}`);
    await delay(250);
  }
  throw new Error('Стенд не вернул доказательство создания и чтения задачи за 35 секунд');
}

async function run(config) {
  const { chromium, expect } = loadPlaywright(config);
  const browser = await chromium.launch({
    headless: true,
    executablePath: config.chromiumExecutable,
    args: ['--disable-dev-shm-usage', '--disable-background-networking'],
  });
  let page;
  const failures = [];
  try {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 1000 },
      locale: 'ru-RU',
      timezoneId: 'Europe/Moscow',
      serviceWorkers: 'block',
      ignoreHTTPSErrors: config.allowLocalSelfSigned === true,
    });
    await context.route('**/*', route => {
      if (new URL(route.request().url()).origin === config.origin) return route.continue();
      return route.abort();
    });
    page = await context.newPage();
    page.setDefaultTimeout(12_000);
    // Only the synthetic session is set by the harness. Every application route is handled by Next.
    const cookieSetup = `${config.origin}/__hitpipe_demo_browser_cookie`;
    await page.route(cookieSetup, route => route.fulfill({
      status: 200,
      headers: { 'Set-Cookie': `${cookieName}=${config.sessionToken}; Path=/; Secure; HttpOnly; SameSite=Strict` },
      body: 'Локальный демонстрационный сеанс',
    }));
    try {
      await page.goto(cookieSetup, { waitUntil: 'domcontentloaded' });
    } finally {
      await page.unroute(cookieSetup);
    }
    const installed = await context.cookies(config.origin);
    assert.ok(installed.some(cookie => cookie.name === cookieName && cookie.value === config.sessionToken),
      'Chromium не принял локальный тестовый сеанс');
    page.on('response', response => {
      const url = new URL(response.url());
      if (response.request().method() === 'POST' && /\/(?:readiness|rehearsals|activate)$/.test(url.pathname)) {
        const time = new Date().toISOString();
        if (response.status() < 400) {
          console.log(`${time} POST ${url.pathname} → ${response.status()}`);
        } else {
          void response.json().then(body => console.log(
            `${time} POST ${url.pathname} → ${response.status()} ${body.code ?? ''}`
          )).catch(() => {});
        }
      }
      if (response.status() >= 400 && url.pathname.startsWith('/api/')) {
        failures.push(`${response.request().method()} ${url.pathname} → ${response.status()}`);
      }
    });
    page.on('pageerror', error => failures.push(`Браузер: ${error.message}`));

    const workspace = `/api/v1/workspaces/${config.workspaceId}`;
    const button = name => page.getByRole('button', { name, exact: true });
    const responseFor = (method, matchesPath) => page.waitForResponse(response => {
      const url = new URL(response.url());
      return url.origin === config.origin && matchesPath(url.pathname) && response.request().method() === method;
    });
    async function clickAndRead(action, matchesPath, expectedStatus, onRejected) {
      const [response] = await Promise.all([responseFor('POST', matchesPath), action.click()]);
      if (response.status() !== expectedStatus) {
        const rejected = await response.json().catch(() => ({}));
        if (onRejected) await onRejected(response, rejected.code);
        throw new Error(`${new URL(response.url()).pathname}: HTTP ${response.status()}, код ${rejected.code ?? 'не указан'}, ожидался ${expectedStatus}`);
      }
      return response.json();
    }

    await page.goto(config.origin, { waitUntil: 'domcontentloaded' });
    await expect(page.getByText('Вход выполнен.', { exact: false })).toBeVisible({ timeout: 15_000 });
    await page.getByRole('link', { name: 'Подготовить операцию', exact: true }).click();
    await expect(page.getByRole('heading', { name: /Подготовка операции/ })).toBeVisible();
    const area = page.getByLabel('Рабочая область настройки');
    if (await area.count()) await area.selectOption(config.workspaceId);
    const stripeConnection = page.getByLabel('Подключение Stripe');
    const clickupConnection = page.getByLabel('Подключение ClickUp');
    // The form cancels pending reads when a user edits it. Wait for real connection metadata.
    const refreshConnections = button('Обновить подключения');
    await expect(refreshConnections).toBeEnabled({ timeout: 12_000 });
    const connectionWarning = page.getByRole('alert').filter({ hasText: 'Сервис временно недоступен (503)' }).first();
    if (await stripeConnection.locator(`option[value="${config.stripeConnectionId}"]`).count() === 0 && await connectionWarning.isVisible()) {
      const [connections] = await Promise.all([
        responseFor('GET', path => path === `${workspace}/connections`), refreshConnections.click(),
      ]);
      assert.equal(connections.status(), 200, 'Повторное чтение подключений должно пройти');
    }
    await expect(stripeConnection.locator(`option[value="${config.stripeConnectionId}"]`)).toBeAttached();
    await expect(clickupConnection.locator(`option[value="${config.clickupConnectionId}"]`)).toBeAttached();

    const operationName = page.getByLabel('Название операции', { exact: true });
    const initialName = config.operationName ?? 'Заказ из Stripe';
    await operationName.fill(initialName);
    await page.getByLabel('Подпись услуги', { exact: true }).fill(config.serviceLabel ?? 'Synthetic service');
    await page.getByLabel('Режим операции').selectOption('TEST');
    await stripeConnection.selectOption(config.stripeConnectionId);
    await clickupConnection.selectOption(config.clickupConnectionId);

    const selections = [
      ['Тариф Stripe (Price)', config.stripeConnectionId, config.priceAlias],
      ['Список ClickUp (List)', config.clickupConnectionId, config.listAlias],
      ['Исполнитель', config.clickupConnectionId, config.assigneeAlias],
    ];
    for (const [label, connectionId, value] of selections) {
      const load = button(`Загрузить: ${label}`);
      const [optionsResponse] = await Promise.all([
        responseFor('GET', path => path === `${workspace}/connections/${connectionId}/options`),
        load.click(),
      ]);
      assert.equal(optionsResponse.status(), 200, `Загрузка «${label}» вернула HTTP ${optionsResponse.status()}`);
      await expect(load).toBeEnabled();
      if (!config.schoolAi) await page.getByRole('combobox', { name: label, exact: true }).selectOption(value);
    }

    const openAssistant = button('Открыть учебного помощника');
    if (config.schoolAi) {
      const proposalPath = '/api/v1/ai/proposals';
      const writes = [];
      const captureWrite = request => {
        const url = new URL(request.url());
        if (request.method() === 'POST' && url.origin === config.origin
            && (url.pathname === proposalPath || url.pathname.startsWith(workspace + '/'))) writes.push(url.pathname);
      };
      page.on('request', captureWrite);
      try {
        await openAssistant.click();
        const assistant = page.getByRole('complementary', { name: 'Учебный помощник' });
        await expect(assistant).toBeVisible();
        await expect(assistant.getByText(/Получатель — локальный учебный помощник/)).toBeVisible();
        const send = assistant.getByRole('button', { name: 'Отправить запрос', exact: true });
        await expect(send).toBeDisabled();
        await assistant.getByRole('button', { name: 'Вставить учебный запрос', exact: true }).click();
        const editor = assistant.getByLabel('Запрос помощнику');
        await expect(editor).toHaveValue('Подготовь тестовую операцию Stripe → ClickUp');
        assert.equal(writes.length, 0, 'Подстановка текста не должна отправлять запрос');

        const [response] = await Promise.all([responseFor('POST', path => path === proposalPath), send.click()]);
        assert.equal(response.status(), 200, 'Защищённый маршрут помощника должен принять отправку');
        assert.equal(await response.headerValue('cache-control'), 'private, no-store');
        const sent = response.request().postDataJSON();
        const requestId = response.request().headers()['x-request-id'];
        assert.ok(uuid.test(requestId) && sent.request_id === requestId && sent.explicit_send === true
          && sent.recipient_notice_version === 'ai-minimal-v1' && sent.editor_text === await editor.inputValue()
          && Array.isArray(sent.intent_aliases) && sent.intent_aliases.length === selections.length
          && selections.every((selection, index) => sent.intent_aliases[index] === selection[2])
          && Array.isArray(sent.synthetic_examples) && sent.synthetic_examples.length === 0
          && Object.keys(sent).length === 6, 'Помощнику ушли данные за пределами разрешённого запроса');
        const proposal = await response.json();
        assert.ok(proposal.status === 'PROPOSAL' && proposal.request_id === requestId && proposal.proposed_name
          && proposal.price_alias === config.priceAlias && proposal.list_alias === config.listAlias
          && proposal.assignee_alias === config.assigneeAlias, 'Получено предложение с неожиданными настройками');

        const apply = assistant.getByRole('button', { name: 'Внести в поля черновика', exact: true });
        await expect(apply).toBeEnabled();
        await expect(operationName).toHaveValue(initialName);
        for (const [label] of selections) {
          await expect(page.getByRole('combobox', { name: label, exact: true })).toHaveValue('');
        }
        await apply.click();
        await expect(operationName).toHaveValue(proposal.proposed_name);
        for (const [label, , value] of selections) {
          await expect(page.getByRole('combobox', { name: label, exact: true })).toHaveValue(value);
        }
        await expect(assistant.getByText(/Предложение внесено в поля/)).toBeVisible();
        await assistant.getByRole('button', { name: 'Скрыть помощника', exact: true }).click();
        await expect(assistant).toHaveCount(0);
        assert.deepEqual(writes, [proposalPath], 'Пользователь ещё не просил сохранить черновик или запустить операцию');
        console.log('Учебный помощник вернул предложение. Поля изменились после нажатия «Внести в поля черновика».');
      } finally {
        page.off('request', captureWrite);
      }
    } else {
      await expect(openAssistant).toHaveCount(0);
    }
    await page.getByLabel('Обязательные ключи контекста через запятую', { exact: true }).fill('');
    await expect(page.getByText('Выбранные варианты подтверждены полученными данными.')).toBeVisible();
    const operations = page.getByRole('region', { name: 'Управление операцией' });
    const readOperations = operations.getByRole('button', { name: 'Прочитать состояние операций', exact: true });
    await expect(readOperations).toBeEnabled({ timeout: 12_000 });
    if (await operations.getByRole('alert').isVisible()) {
      const problem = await operations.getByRole('alert').innerText();
      assert.match(problem, /\(503\)|Истекло время ожидания ответа/, 'При чтении операций произошла другая ошибка');
      const [current] = await Promise.all([
        responseFor('GET', path => path === `${workspace}/operations`), readOperations.click(),
      ]);
      assert.equal(current.status(), 200, 'Повторное чтение операций должно пройти');
    }
    await expect(operations.getByText('Сервис не вернул операций.')).toBeVisible();
    const draft = await clickAndRead(button('Сохранить черновик'), path => path === `${workspace}/drafts`, 201);
    assert.ok(typeof draft.draft_id === 'string' && uuid.test(draft.draft_id) && draft.revision === 1,
      'Владелец не сохранил черновик ревизии 1');
    await page.screenshot({ path: join(config.artifactDir, '01-before-activation.png'), fullPage: true });
    const readiness = await clickAndRead(
      button('Проверить готовность'), path => path === `${workspace}/drafts/${draft.draft_id}/readiness`, 200,
    );
    assert.equal(readiness.passed, true, 'Проверка готовности операции должна пройти');
    assert.ok(/^sha256:[a-f0-9]{64}$/.test(readiness.activation_preview?.accepted_config_digest),
      'Владелец должен предоставить точный снимок конфигурации для активации');

    const rehearsal = await clickAndRead(
      button('Запустить репетицию'), path => path === `${workspace}/rehearsals`, 200,
    );
    assert.equal(rehearsal.live_requests_made, 0, 'Репетиция не должна вызывать внешние сервисы');
    await expect(page.getByText('Внешних запросов: 0.', { exact: false })).toBeVisible();
    await operations.getByRole('button', { name: 'Просмотреть конфигурацию для включения' }).click();
    const activationButton = operations.getByRole('button', { name: 'Включить эту операцию', exact: true });
    await expect(activationButton).toBeVisible();
    const activationDelayMs = Number(process.env.HITPIPE_ACTIVATION_DELAY_MS ?? '0');
    assert.ok(Number.isInteger(activationDelayMs) && activationDelayMs >= 0 && activationDelayMs <= 10000);
    if (activationDelayMs) await page.waitForTimeout(activationDelayMs);
    const operation = await clickAndRead(
      activationButton, path => path === `${workspace}/operations/activate`, 201, async (response, code) => {
        const command = response.request().postDataJSON();
        const recordedAt = new Date().toISOString();
        const diagnostic = {
          http_status: response.status(),
          code,
          recorded_at: recordedAt,
          command: {
            draft_id: command.draft_id, draft_revision: command.draft_revision,
            readiness_check_id: command.readiness_check_id, rehearsal_id: command.rehearsal_id,
            accepted_config_digest: command.accepted_config_digest, acceptance_text_version: command.acceptance_text_version,
            confirm_activate: command.confirm_activate, expected_revision: command.command?.expected_revision,
          },
          draft: { draft_id: draft.draft_id, revision: draft.revision, required_context_keys: draft.required_context_keys },
          readiness: {
            check_id: readiness.check_id, draft_revision: readiness.draft_revision,
            checked_at: readiness.checked_at, expires_at: readiness.expires_at,
            age_ms: Date.parse(recordedAt) - Date.parse(readiness.checked_at),
            remaining_ms: Date.parse(readiness.expires_at) - Date.parse(recordedAt),
            config_digest: readiness.config_digest,
            accepted_config_digest: readiness.activation_preview.accepted_config_digest,
          },
          rehearsal: {
            rehearsal_id: rehearsal.rehearsal_id, draft_revision: rehearsal.draft_revision,
            config_digest: rehearsal.config_digest, live_requests_made: rehearsal.live_requests_made, results: rehearsal.results,
          },
        };
        await writeFile(join(config.artifactDir, 'activation-failure.json'), JSON.stringify(diagnostic, null, 2) + '\n', { mode: 0o600 });
        console.error('Диагностика отклонённой активации:', JSON.stringify(diagnostic));
      },
    );
    assert.equal(operation.state, 'ACTIVE');
    assert.equal(operation.accepted_config_digest, readiness.activation_preview.accepted_config_digest);
    assert.ok(typeof operation.operation_id === 'string' && uuid.test(operation.operation_id), 'Принятая операция без ID');
    await expect(operations.getByText(/Состояние: .*\(ACTIVE\)/)).toBeVisible();
    await page.screenshot({ path: join(config.artifactDir, '02-operation-active.png'), fullPage: true });
    console.log('Браузер подтвердил черновик, готовность, репетицию и состояние ACTIVE.');

    if (config.stripeTriggerUrl) {
      const trigger = await fetch(config.stripeTriggerUrl, {
        method: 'POST', signal: AbortSignal.timeout(10_000), redirect: 'error',
      });
      assert.ok(trigger.ok, `Локальная подача события Stripe вернула HTTP ${trigger.status}`);
    }
    const proof = await proofOfDelivery(config.clickupProofUrl);
    if (proof.operation_id !== undefined) assert.equal(proof.operation_id, operation.operation_id);
    if (proof.case_state !== undefined) assert.equal(proof.case_state, 'CONFIRMED');
    const [listResponse] = await Promise.all([
      responseFor('GET', path => path === `${workspace}/cases`),
      page.getByRole('link', { name: 'К списку дел', exact: true }).click(),
    ]);
    assert.equal(listResponse.status(), 200, 'Рабочая область должна прочитать дела через BFF');
    const list = await listResponse.json();
    assert.equal(list.items.length, 1, 'Дубликат исходного события не должен создавать второе дело');
    assert.equal(list.items[0].case_id, proof.case_id, 'В браузере должно быть то же дело, что обработал стенд');
    assert.equal(list.items[0].operation_id, operation.operation_id, 'Дело должно принадлежать только что активированной операции');
    assert.equal(list.items[0].state, 'CONFIRMED');
    const result = page.getByRole('region', { name: 'Список дел' }).getByRole('button', { name: /Подтверждено/ });
    await expect(result).toHaveCount(1);
    const [detailResponse] = await Promise.all([
      responseFor('GET', path => path === `${workspace}/cases/${proof.case_id}`),
      result.click(),
    ]);
    assert.equal(detailResponse.status(), 200, 'Браузер должен прочитать детали подтверждённого дела');
    const detail = await detailResponse.json();
    assert.equal(detail.case.case_id, proof.case_id);
    assert.equal(detail.case.operation_id, operation.operation_id);
    assert.equal(detail.case.state, 'CONFIRMED');
    assert.equal(detail.case.outcome_knowledge, 'RESULT_VERIFIED');
    assert.equal(detail.observation_complete, true);
    assert.ok(Number.isFinite(Date.parse(detail.last_observation_at)), 'Детали должны содержать время чтения ClickUp');

    const details = page.getByRole('region', { name: 'Деталь дела' });
    await expect(details.getByTestId('status-badge')).toHaveText('Подтверждено');
    await expect(details.getByText('Результат проверен', { exact: true })).toBeVisible();
    await expect(details.getByTestId('field-case-id')).toHaveText(proof.case_id);
    const link = details.getByRole('link', { name: 'Открыть подтверждённый результат', exact: true });
    await expect(link).toBeVisible();
    const href = new URL(await link.getAttribute('href'));
    assert.equal(href.protocol, 'https:');
    assert.equal(decodeURIComponent(href.pathname.split('/').filter(Boolean).at(-1)), proof.task_id,
      'Ссылка результата должна указывать на задачу, которую прочитал Delivery');
    await page.screenshot({ path: join(config.artifactDir, '03-case-confirmed.png'), fullPage: true });

    await writeFile(join(config.artifactDir, 'browser-proof.json'), JSON.stringify({
      browser: chromium.name(),
      browser_version: browser.version(),
      ...(config.schoolAi ? { assistant_proposal_applied: true } : {}),
      operation_id: operation.operation_id,
      case_id: proof.case_id,
      clickup_task_id: proof.task_id,
      clickup_title: proof.title,
      clickup_create_count: proof.create_count,
      clickup_read_count: proof.read_count,
      case_state: detail.case.state,
      outcome_knowledge: detail.case.outcome_knowledge,
      last_observation_at: detail.last_observation_at,
    }, null, 2) + '\n', { mode: 0o600 });
    console.log('Браузер показал «Подтверждено». Ссылка совпала с единственной созданной и прочитанной задачей ClickUp.');
  } catch (error) {
    if (page) {
      const alerts = await page.getByRole('alert').allTextContents().catch(() => []);
      if (alerts.length) console.error(`Сообщения страницы: ${alerts.slice(0, 4).join(' | ')}`);
      await page.screenshot({ path: join(config.artifactDir, 'browser-failure.png'), fullPage: true }).catch(() => {});
    }
    for (const failure of failures.slice(-8)) console.error(failure);
    throw error;
  } finally {
    await browser.close();
  }
}

const configPath = process.argv[2];
assert.ok(configPath, 'Передайте абсолютный путь к локальному JSON-конфигу браузера');
const config = validateConfig(JSON.parse(await readFile(configPath, 'utf8')));
await mkdir(config.artifactDir, { recursive: true, mode: 0o700 });
try {
  await run(config);
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
