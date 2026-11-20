package readcases

import (
	"encoding/json"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lead HIT-95/12640, CCR revision3. These are display explanations, never command
// authority. Only a qualified server-side availability check enables RECHECK.
func Project(r Record) (Labels, error) { return project(r, commandStatus{}) }

// ProjectRecovery keeps list projections useful without treating a workspace
// read grant as authority for every case. Only a qualified detail enables RECHECK.
func ProjectRecovery(r Record, available bool) (Labels, error) {
	known := r.KnownTaskID != nil && *r.KnownTaskID != ""
	return project(r, commandStatus{implemented: true, authorityKnown: available, authorized: available,
		checksKnown: available, checksPassed: available, sameIdentity: known})
}

type commandStatus struct{ implemented, authorityKnown, authorized, checksKnown, checksPassed, sameIdentity bool }
type reasonInfo struct {
	text           string
	accessObstacle bool
}

// Only actual stored producer codes are described. In particular READER_UNAVAILABLE
// is not fabricated proof of a denied connection. No current producer persists a
// specific access-denial reason; the conditional priority6 remains conservative.
var reasonCatalog = map[string]reasonInfo{
	"RECHECK_ACCEPTED":            {"Запрос на повторную проверку принят.", false},
	casetimeouts.WorkingElapsed:   {"Истёк срок обработки. Внешний исход требует проверки.", false},
	casetimeouts.WaitingElapsed:   {"Проверенные данные не поступили за 24 часа непрерывного ожидания.", false},
	casetimeouts.HistoryMissing:   {"Не найдена запись о начале текущего состояния. Действие удержано.", false},
	casetimeouts.HistoryInvalid:   {"Данные о начале текущего состояния недоступны или несогласованны. Действие удержано.", false},
	casetimeouts.HistoryExpired:   {"Подробности начала текущего состояния просрочены. Действие удержано.", false},
	casetimeouts.EpisodeMissing:   {"Не найден открытый эпизод обработки. Действие удержано.", false},
	casetimeouts.EpisodeInvalid:   {"Времена или связь эпизода обработки несогласованны. Действие удержано.", false},
	"DELIVERY_RESULT_VERIFIED":    {"Результат проверен Delivery.", false},
	"DELIVERY_TARGET_ID_OBSERVED": {"Наблюдался ID задачи.", false},
	"DELIVERY_POSSIBLE_EFFECT":    {"Внешняя запись могла состояться.", false},
	"DELIVERY_RESULT_MISMATCH":    {"Наблюдалось несоответствие задачи.", false},
	"DELIVERY_INCOMPLETE":         {"Проверка назначения неполна.", false},
	"DELIVERY_KNOWN_NO_EFFECT":    {"Delivery установил отсутствие эффекта.", false},
	"DELIVERY_OBSERVATION_HELD":   {"Наблюдение не усилило знание.", false},
	"DELIVERY_OBSERVATION_LATE":   {"Сохранено позднее наблюдение.", false},
	"DELIVERY_IDENTITY_CONFLICT":  {"ID назначения не совпал.", false},
	"SIGNAL_ADMITTED":             {"Дело принято.", false},
	"FINANCIAL_ELIGIBILITY_ONLY":  {"Финансовые условия проверены.", false},
	"CONTEXT_UNAVAILABLE":         {"Обязательный контекст непригоден.", false},
	"RENDERING_OR_PROOF_INVALID":  {"Подготовка данных удержана.", false},
	"READER_UNAVAILABLE":          {"Данные источника недоступны.", false},
	"INVALID_CONTEXT":             {"Контекст проверки непригоден.", false},
	"PIN_MISMATCH":                {"Версия данных не совпала.", false},
	"IDENTITY_MISMATCH":           {"Идентичность источника не совпала.", false},
	"SOURCE_SCOPE":                {"Область источника не совпала.", false},
	"UNKNOWN_FACT":                {"Необходимые факты не установлены.", false},
	"INVALID_INPUT":               {"Вход проверки непригоден.", false},
	"INVALID_FACT":                {"Факт источника некорректен.", false},
	"PROOF_TIMELINE":              {"Времена доказательства непригодны.", false},
	"PROOF_EXPIRED":               {"Доказательство устарело.", false},
	"REFUND":                      {"Зафиксирован возврат.", false},
	"DISPUTE":                     {"Зафиксирован спор.", false},
	"PAYMENT_STATUS":              {"Оплата не подтверждена.", false},
	"MODE":                        {"Режим оплаты не подходит.", false},
	"LINE_ITEM_COUNT":             {"Состав заказа не подходит.", false},
	"PRICE":                       {"Цена не совпала.", false},
	"QUANTITY":                    {"Количество не подходит.", false},
	"CURRENCY":                    {"Валюты не совпали.", false},
	"AMOUNT_RECEIVED":             {"Полученная сумма не совпала.", false},
	"AMOUNT_CAPTURED":             {"Захваченная сумма не совпала.", false},
	"NON_POSITIVE_TOTAL":          {"Сумма заказа неположительна.", false},
	"PAYMENT_INTENT":              {"Платёж не завершён.", false},
	"CHARGE_FAILED":               {"Оплата неуспешна.", false},
	"NOT_CAPTURED":                {"Захват оплаты не подтверждён.", false},
	"BEFORE_ACTIVATION":           {"Заказ создан до первой активации.", false},
}
var stateText = map[string]string{
	"RECEIVED":                    "Дело получено, ожидается проверка данных.",
	"WAITING_DATA":                "Ожидаются необходимые данные.",
	"WORKING":                     "Дело находится в обработке.",
	"UNKNOWN":                     "Результат внешнего действия не установлен.",
	"ATTENTION":                   "Есть препятствие, требующее разбора.",
	"CONFIRMED":                   "Результат подтверждён последней сохранённой проверкой.",
	"EXCLUDED":                    "Дело исключено из обработки.",
	"CLOSED_WITHOUT_CONFIRMATION": "Дело закрыто без подтверждения результата.",
}

func ContextComplete(r Record) bool {
	if r.ContextExpired || len(r.Context) != len(r.RequiredKeys) {
		return false
	}
	values := map[string]string{}
	for _, f := range r.Context {
		values[f.Key] = f.Value
	}
	for _, key := range r.RequiredKeys {
		_, ok := values[key]
		if !ok {
			return false
		}
	}
	return true
}
func nextStep(state, outcome string, fenced, complete, access, canRecheck bool) (string, int) {
	switch {
	case canRecheck:
		return "Доступна повторная проверка известной задачи ClickUp.", 1
	case state == "UNKNOWN" || outcome == "POSSIBLE_EFFECT" || (outcome == "UNESTABLISHED" && fenced):
		return "Нужно установить внешний результат. Повторное создание задачи запрещено.", 2
	case outcome == "RESULT_MISMATCH":
		return "Нужно разобрать несоответствие результата. Повторное создание задачи недоступно.", 3
	case state == "CONFIRMED" || state == "EXCLUDED" || state == "CLOSED_WITHOUT_CONFIRMATION":
		return "Дополнительных действий с делом в этой версии нет.", 4
	case !complete:
		return "Нужны актуальные обязательные данные; новая отправка удержана.", 5
	case access:
		return "Владельцу нужно восстановить доступ к прежнему подключению.", 6
	case state == "WAITING_DATA":
		return "Ожидаются проверенные данные. Ручное добавление данных в этой версии недоступно.", 7
	case state == "RECEIVED" || state == "WORKING":
		return "Дождитесь результата обработки.", 8
	default:
		return "Требуется разбор сохранённой причины препятствия.", 9
	}
}
func project(r Record, c commandStatus) (Labels, error) {
	var cfg map[string]json.RawMessage
	if json.Unmarshal(r.Config, &cfg) != nil {
		return Labels{}, ErrUnavailable
	}
	var label string
	if json.Unmarshal(cfg["service_label"], &label) != nil || !utf8.ValidString(label) || utf8.RuneCountInString(label) < 1 || utf8.RuneCountInString(label) > 120 {
		return Labels{}, ErrUnavailable
	}
	for _, v := range label {
		if unicode.IsControl(v) {
			return Labels{}, ErrUnavailable
		}
	}
	base, ok := stateText[r.Summary.State]
	if !ok {
		return Labels{}, ErrUnavailable
	}
	outcome := r.Summary.OutcomeKnowledge
	switch outcome {
	case "NO_EFFECT", "POSSIBLE_EFFECT", "TARGET_ID_KNOWN", "RESULT_VERIFIED", "RESULT_MISMATCH", "UNESTABLISHED":
	default:
		return Labels{}, ErrUnavailable
	}
	reason, ok := reasonCatalog[r.LatestReason]
	if !ok {
		reason.text = "Сохранённая причина не расшифрована."
	}
	known := r.KnownTaskID != nil && *r.KnownTaskID != ""
	ready := c.implemented && known && c.sameIdentity && c.authorityKnown && c.authorized && c.checksKnown && c.checksPassed
	step, _ := nextStep(r.Summary.State, outcome, r.DispatchFenced, ContextComplete(r), reason.accessObstacle, ready)
	parts := []string{base, step, reason.text}
	actions := []string{}
	if ready {
		actions = append(actions, "RECHECK")
	} else {
		switch {
		case !c.implemented:
			parts = append(parts, "Повторная проверка в этой версии не подключена.")
		case !known || !c.sameIdentity:
			parts = append(parts, "Прежняя задача для проверки не установлена.")
		case !c.authorityKnown || !c.checksKnown:
			parts = append(parts, "Доступность повторной проверки не установлена.")
		case !c.authorized:
			parts = append(parts, "Прав на повторную проверку нет.")
		default:
			parts = append(parts, "Проверки повторного действия не выполнены.")
		}
	}
	if (r.Summary.State == "UNKNOWN" || outcome == "POSSIBLE_EFFECT" || r.DispatchFenced) && !strings.Contains(step, "Повторное создание задачи") {
		parts = append(parts, "Повторное создание задачи запрещено.")
	}
	switch outcome {
	case "TARGET_ID_KNOWN":
		parts = append(parts, "Известен ID задачи; подтверждения результата нет.")
	case "RESULT_VERIFIED":
		parts = append(parts, "Указана последняя проверка, не текущее состояние ClickUp.")
	case "UNESTABLISHED":
		if !r.DispatchFenced {
			parts = append(parts, "Знание о результате не установлено.")
		}
	}
	text := strings.Join(parts, " ")
	if utf8.RuneCountInString(text) > 512 {
		return Labels{}, ErrUnavailable
	}
	return Labels{OrderLabel: label, NextAction: text, Actions: actions}, nil
}
