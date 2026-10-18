# Человеческий review и merge

HUMAN_PENDING. Кодовые PR: https://github.com/hitpipe/hitpipe/pull/5 (HIT-27), /pull/6 (HIT-28), /pull/7 (HIT-29). Требования /pull/3 и /pull/4. Bootstrap /pull/2 ещё открыт на момент чтения. Все зависимые ветки явно stacked; parent не объявлен merged. D1 и H1/REPORT PR регистрируются в handoff после действительного создания.

Lead организует независимого разрешённого human reviewer. Проверить contract/source pins, financial predicates и null semantics, consumer pin/error handling, safe diagnostics, assertions/mutation sensitivity, logs и ограничения. Реальные remarks сохраняются в PR, автор исправляет и повторяет relevant checks. Merge только человеком после реального approval/current CI. После merge parent ретаргетировать/rebase собственные дочерние ветки на действительную новую базу и повторно проверить CI; не force-push чужую ветку и не считать stacked commit merged заранее.

Source contract acceptance, formal Development admission и official /review backend не выполнялись. Для immutable lab tags требуются actual merge и проверенный финальный SHA; запись tag→SHA→CI→report digests в внешней submission record после tag, без собственного SHA в report commit. Теги не создавать/не переносить до этих условий. Защита/оценка/подписи преподавателя человеком, здесь не заполнены.
