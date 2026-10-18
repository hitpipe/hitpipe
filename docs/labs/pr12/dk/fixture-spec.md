# Синтетические входы и независимые ожидаемые исходы

Версия: R1/HIT-17, до реализации. Общие константы: created `2026-10-18T00:00:00Z`, первоначальный idle `2026-10-18T08:00:00Z`, absolute `2026-10-19T00:00:00Z`. Наносекундные границы обрабатываются в HIT-18.

| Case ID | Изменение относительно корректного snapshot | Ожидаемый outcome / основание |
| --- | --- | --- |
| DK-NORMAL | known, not revoked, now=07:59:59Z, trusted=true | Valid; до обоих сроков, T/002 |
| DK-UNKNOWN | found=false | Denied; неизвестность не даёт доступ |
| DK-REVOKED | revoked=true, сроки в будущем | Denied; отзыв сильнее прежнего успеха |
| DK-IDLE | now=08:00:01Z | Denied; idle истёк |
| DK-ABSOLUTE | idle=absolute, now=2026-10-19T00:00:01Z | Denied; абсолютный срок истёк |
| DK-CHRONOLOGY | expires_at раньше created_at | Denied; некорректные authority facts |
| DK-CLOCK | trusted=false | Denied/Unavailable в соответствующей границе; ни одного Valid |
| DK-PARSE | malformed JSON, extra/duplicate key, null required field, trailing document | Config error и ненулевой CLI exit |

Это авторские oracle из правил, а не записанный вывод будущей программы. Данные не содержат реальных session keys, cookies, customer IDs или secrets. Fixtures C1 будут read-only относительно программы; повторный запуск не изменяет исходные данные и не требует reset внешней системы.
