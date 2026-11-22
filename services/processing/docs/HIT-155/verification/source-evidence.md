# Происхождение проверенного исходника HIT-155

Codex по делегации Тимофея Патрушева. Фактическая проверка Git из рабочего дерева интеграции.

| Источник | Проверенный исходник | Результат и ветка |
| --- | --- | --- |
| HIT-145 | `30385d09962c3fa4aad4808253a5493235559d01` | [PASS WITH NOTES 4/4](https://hitpipe.atlassian.net/browse/HIT-145?focusedCommentId=15704), [Draft PR №84](https://github.com/hitpipe/hitpipe/pull/84) `4a551a9bb17c0961562b51ceb85bb1e53862bbde` |
| HIT-128 | `c426dc400a4bec03fafba72e2a5780c9d85af1e2` | [PASS 6/6](https://hitpipe.atlassian.net/browse/HIT-128?focusedCommentId=15934), [Draft PR №85](https://github.com/hitpipe/hitpipe/pull/85) `8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3` |

Общий предок `49229e1db3f371a051a022ea9e31951dd558eecb`. Merge `d2083e0317c507e1ac32180b6f156003969e04cd` свёл обе ветки без конфликтов. Ни один проверенный SHA зависимости не входит в наблюдаемый `main` `ce771d37505e7770003d0796ae932556d06b7bab`. Интеграционный исполняемый исходник `1ed91653a747b47a98f2738ca0d831a38cebc98a`. Git подтвердил сохранение обеих принятых версий и неизменность производственных файлов после merge.

[Записи Git с кодами выхода и хешами](source-commands.json). Для полного двоичного патча по каждому родителю использованы `git diff --full-index --binary <родитель> <исходник>`. SHA-256 рассчитан по исходным байтам Git. GitHub также показывает сравнение закреплённых ревизий.

- HIT-145 · 5546680 байт · SHA-256 `1622e54443af2e1f56fafd421a1cf8f05c58e37eac72a80cb8dc6ef4c5fb425e`. [Сравнение GitHub](https://github.com/hitpipe/hitpipe/compare/4a551a9bb17c0961562b51ceb85bb1e53862bbde...1ed91653a747b47a98f2738ca0d831a38cebc98a).
- HIT-128 · 303450 байт · SHA-256 `d85930a00ff435b4010ebcc98f985e56589fb48df3f80c31deef1d8066c24266`. [Сравнение GitHub](https://github.com/hitpipe/hitpipe/compare/8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3...1ed91653a747b47a98f2738ca0d831a38cebc98a).

## Фактические команды связи с принятой базой

`git rev-parse HEAD` → exit 0. Вывод `1ed91653a747b47a98f2738ca0d831a38cebc98a`.

`git merge-base --all 4a551a9bb17c0961562b51ceb85bb1e53862bbde 8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3` → exit 0. Вывод `49229e1db3f371a051a022ea9e31951dd558eecb`.

`git show -s --format=%P d2083e0317c507e1ac32180b6f156003969e04cd` → exit 0. Вывод `8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3 4a551a9bb17c0961562b51ceb85bb1e53862bbde`.

`git merge-base --is-ancestor 30385d09962c3fa4aad4808253a5493235559d01 4a551a9bb17c0961562b51ceb85bb1e53862bbde` → exit 0.

`git merge-base --is-ancestor 30385d09962c3fa4aad4808253a5493235559d01 d2083e0317c507e1ac32180b6f156003969e04cd` → exit 0.

`git merge-base --is-ancestor 30385d09962c3fa4aad4808253a5493235559d01 1ed91653a747b47a98f2738ca0d831a38cebc98a` → exit 0.

`git merge-base --is-ancestor 30385d09962c3fa4aad4808253a5493235559d01 ce771d37505e7770003d0796ae932556d06b7bab` → exit 1.

`git merge-base --is-ancestor c426dc400a4bec03fafba72e2a5780c9d85af1e2 8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3` → exit 0.

`git merge-base --is-ancestor c426dc400a4bec03fafba72e2a5780c9d85af1e2 d2083e0317c507e1ac32180b6f156003969e04cd` → exit 0.

`git merge-base --is-ancestor c426dc400a4bec03fafba72e2a5780c9d85af1e2 1ed91653a747b47a98f2738ca0d831a38cebc98a` → exit 0.

`git merge-base --is-ancestor c426dc400a4bec03fafba72e2a5780c9d85af1e2 ce771d37505e7770003d0796ae932556d06b7bab` → exit 1.

`git rev-parse 4a551a9bb17c0961562b51ceb85bb1e53862bbde:services/processing/Dockerfile` → exit 0. Вывод `45fc51d1959d699def33941ba0464fdb18559274`.

`git rev-parse 1ed91653a747b47a98f2738ca0d831a38cebc98a:services/processing/Dockerfile` → exit 0. Вывод `45fc51d1959d699def33941ba0464fdb18559274`.

`git rev-parse 4a551a9bb17c0961562b51ceb85bb1e53862bbde:services/processing/Dockerfile.dockerignore` → exit 0. Вывод `0ae75640ef095ea264eea9af2dbb231c02591553`.

`git rev-parse 1ed91653a747b47a98f2738ca0d831a38cebc98a:services/processing/Dockerfile.dockerignore` → exit 0. Вывод `0ae75640ef095ea264eea9af2dbb231c02591553`.

`git rev-parse 4a551a9bb17c0961562b51ceb85bb1e53862bbde:.project-os/results/HIT-145.json` → exit 0. Вывод `471a7df7725edfc0eeace0cfc05ac6708fa21a22`.

`git rev-parse 1ed91653a747b47a98f2738ca0d831a38cebc98a:.project-os/results/HIT-145.json` → exit 0. Вывод `471a7df7725edfc0eeace0cfc05ac6708fa21a22`.

`git rev-parse 8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3:services/processing/scripts/recovery/verify_recovery.py` → exit 0. Вывод `c6d6a29a173e2b3444f7b7d0d780dcc642b6dc13`.

`git rev-parse 1ed91653a747b47a98f2738ca0d831a38cebc98a:services/processing/scripts/recovery/verify_recovery.py` → exit 0. Вывод `c6d6a29a173e2b3444f7b7d0d780dcc642b6dc13`.

`git rev-parse 8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3:services/processing/scripts/migrations/001_processing.sql` → exit 0. Вывод `87c321d67376e450cab3d6b1cb551e52201e0f3e`.

`git rev-parse 1ed91653a747b47a98f2738ca0d831a38cebc98a:services/processing/scripts/migrations/001_processing.sql` → exit 0. Вывод `87c321d67376e450cab3d6b1cb551e52201e0f3e`.

`git rev-parse 8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3:.project-os/results/HIT-128.json` → exit 0. Вывод `377fdb8adb9ed6e2c75f8b5f9496677ed49b332e`.

`git rev-parse 1ed91653a747b47a98f2738ca0d831a38cebc98a:.project-os/results/HIT-128.json` → exit 0. Вывод `377fdb8adb9ed6e2c75f8b5f9496677ed49b332e`.

## Точный патч адаптации стенда

Изменены только синтетические fixtures, запускающие тесты и проверяющие образ. Сохранились байты Dockerfile, миграции и исходного runner HIT-128. [Воспроизведение расхождения](first-attempts.md).

```diff
diff --git a/services/processing/internal/adapters/postgres/image_process_test.go b/services/processing/internal/adapters/postgres/image_process_test.go
index eb21029e07ec50060a15c0af0aef7b308e2cd6cc..f52ed9d0e148a083614a5e74a55141b5f98fdf35 100644
--- a/services/processing/internal/adapters/postgres/image_process_test.go
+++ b/services/processing/internal/adapters/postgres/image_process_test.go
@@ -15,0 +16 @@ import (
+	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
@@ -71,0 +73,23 @@ type imageState struct {
+func awaitImageHistory(t *testing.T, stand *processStand, path string, headers map[string]string) []byte {
+	t.Helper()
+	// The integrated runtime has a case worker. Its controlled Intake answers
+	// 503, so compare history only after the expected durable hold is recorded.
+	deadline := time.Now().Add(8 * time.Second)
+	for time.Now().Before(deadline) {
+		body := stand.must("bff.test", "8443", "GET", path, nil, headers, 200)
+		var history readcases.HistoryView
+		if err := json.Unmarshal(body, &history); err != nil {
+			t.Fatal(err)
+		}
+		for _, item := range history.Items {
+			if item.NewState == "WAITING_DATA" && item.ReasonCode == "READER_UNAVAILABLE" {
+				t.Log("baseline history captured after the controlled Intake failure was durably recorded")
+				return body
+			}
+		}
+		time.Sleep(40 * time.Millisecond)
+	}
+	t.Fatal("case worker did not persist the expected Intake hold")
+	return nil
+}
+
@@ -144 +168 @@ func TestProcessingOCIImageOwnedStand(t *testing.T) {
-	history := stand.must("bff.test", "8443", "GET", historyURL, nil, own, 200)
+	history := awaitImageHistory(t, stand, historyURL, own)
@@ -184,3 +208,8 @@ func TestProcessingOCIImageOwnedStand(t *testing.T) {
-	if json.Unmarshal(history, &body) != nil || json.Unmarshal(again, &secondBody) != nil ||
-		len(body.Items) == 0 || !reflect.DeepEqual(body.Items, secondBody.Items) || !bytes.Equal(receipt, repeated) {
-		t.Fatal("history or admission receipt was not preserved across restart")
+	if !bytes.Equal(receipt, repeated) {
+		t.Fatalf("admission receipt changed across restart: before=%x after=%x", sha256.Sum256(receipt), sha256.Sum256(repeated))
+	}
+	if json.Unmarshal(history, &body) != nil || json.Unmarshal(again, &secondBody) != nil || len(body.Items) == 0 {
+		t.Fatal("expected nonempty history before and after the restart")
+	}
+	if !reflect.DeepEqual(body.Items, secondBody.Items) {
+		t.Fatalf("synthetic history changed across restart: before=%s after=%s", marshalProcess(t, body.Items), marshalProcess(t, secondBody.Items))
diff --git a/services/processing/internal/adapters/postgres/observations_test.go b/services/processing/internal/adapters/postgres/observations_test.go
index e06f0e97090e482030e22abc7c3420670ec5b8df..7abd678a2273742da49a11a54ffbd7d29d8069b7 100644
--- a/services/processing/internal/adapters/postgres/observations_test.go
+++ b/services/processing/internal/adapters/postgres/observations_test.go
@@ -74 +74,5 @@ func newObservation(t *testing.T) observationFixture {
-	f := prepared(t, []string{})
+	return observePrepared(t, prepared(t, []string{}))
+}
+
+func observePrepared(t *testing.T, f preparedFixture) observationFixture {
+	t.Helper()
diff --git a/services/processing/internal/adapters/postgres/oci_recovery_process_test.go b/services/processing/internal/adapters/postgres/oci_recovery_process_test.go
new file mode 100644
index 0000000000000000000000000000000000000000..d93c907457ab03d94ed5541e18336c8c8192f645
--- /dev/null
+++ b/services/processing/internal/adapters/postgres/oci_recovery_process_test.go
@@ -0,0 +1,40 @@
+package postgres
+
+import (
+	"os"
+	"strconv"
+	"testing"
+	"time"
+)
+
+// The driver owns PostgreSQL and fixtures, never a Docker socket. The host
+// acknowledges each lifecycle step after checking the actual OCI container.
+func TestProcessingOCIRecoveryOwnedStand(t *testing.T) {
+	revision, purge := os.Getenv("HIT155_SOURCE_SHA"), os.Getenv("HIT126_PURGE_BIN")
+	if revision == "" || purge == "" || os.Getenv("HIT78_ISOLATED_PG") != "1" || os.Getenv("HIT128_RECOVERY_STAND") != "1" {
+		t.Skip("requires the private HIT-155 OCI recovery runner")
+	}
+	if os.Geteuid() != 0 {
+		t.Fatal("the fixture driver requires root to initialize the disposable restore cluster")
+	}
+	activation := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
+	f := observePrepared(t, preparedIn(t, []string{}, activation, imageContent))
+	verifyProcessingRecovery(t, f, "", purge, func(stand *processStand) recoveryDriver {
+		step := 0
+		signal := func(operation string, args []string) {
+			t.Helper()
+			step++
+			index := strconv.Itoa(step)
+			writeImageSignal(t, "recovery-"+index+".json", map[string]any{"operation": operation, "args": args})
+			awaitImageSignal(t, "recovery-ack-"+index)
+		}
+		return recoveryDriver{
+			boot: func(_ string, args ...string) { signal("start", args) },
+			health: func(label, state string) {
+				t.Log(label)
+				imageHealth(t, stand, state, revision)
+			},
+			stop: func() { signal("kill", nil) },
+		}
+	})
+}
diff --git a/services/processing/internal/adapters/postgres/prepare_test.go b/services/processing/internal/adapters/postgres/prepare_test.go
index 6675596bdfd83ad50ed92c1b920892c1f457a403..894a9d96680ad50c4bfbadb93f0c878c8dd405dc 100644
--- a/services/processing/internal/adapters/postgres/prepare_test.go
+++ b/services/processing/internal/adapters/postgres/prepare_test.go
@@ -155,0 +156,9 @@ func preparedSince(t *testing.T, keys []string, activation time.Time) preparedFi
+	t.Helper()
+	path := filepath.Join(t.TempDir(), "content")
+	if err := os.Mkdir(path, 0700); err != nil {
+		t.Fatal(err)
+	}
+	return preparedIn(t, keys, activation, path)
+}
+
+func preparedIn(t *testing.T, keys []string, activation time.Time, path string) preparedFixture {
@@ -181,4 +189,0 @@ func preparedSince(t *testing.T, keys []string, activation time.Time) preparedFi
-	path := filepath.Join(t.TempDir(), "content")
-	if e = os.Mkdir(path, 0700); e != nil {
-		t.Fatal(e)
-	}
diff --git a/services/processing/internal/adapters/postgres/recovery_process_test.go b/services/processing/internal/adapters/postgres/recovery_process_test.go
index 7d604df9c7c652c9aa60f3ab5684b0a2e97f58a8..3a01a76107c006bb6794f122445a10262794d736 100644
--- a/services/processing/internal/adapters/postgres/recovery_process_test.go
+++ b/services/processing/internal/adapters/postgres/recovery_process_test.go
@@ -12,0 +13 @@ import (
+	"sync/atomic"
@@ -150,0 +152,15 @@ type recoveryOutbox struct {
+type recoveryDriver struct {
+	boot   func(log string, args ...string)
+	health func(label, state string)
+	stop   func()
+}
+
+func localRecoveryDriver(stand *processStand) recoveryDriver {
+	var child *processingChild
+	return recoveryDriver{
+		boot:   func(log string, args ...string) { child = stand.bootWith(log, args...) },
+		health: func(label, state string) { stand.recoveryHealth(stand.t, child, label, state) },
+		stop:   func() { stopRecoveryProcess(stand.t, child) },
+	}
+}
+
@@ -188,0 +205,5 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+	verifyProcessingRecovery(t, newObservation(t), binary, purge, localRecoveryDriver)
+}
+
+func verifyProcessingRecovery(t *testing.T, f observationFixture, binary, purge string, driver func(*processStand) recoveryDriver) {
+	t.Helper()
@@ -190 +210,0 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	f := newObservation(t)
@@ -194,0 +215,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+	runtime := driver(stand)
+	bootStand := func(log string) { runtime.boot(log, "-config", stand.config, "-stand-config", stand.stand) }
@@ -195,0 +218 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+	var deliveryQueries atomic.Int32
@@ -197,0 +221 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+		deliveryQueries.Add(1)
@@ -200,2 +224,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	proc := stand.boot("recovery-source.log")
-	stand.recoveryHealth(t, proc, "original", "CLOSED")
+	bootStand("recovery-source.log")
+	runtime.health("original", "CLOSED")
@@ -203 +227 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stand.recoveryHealth(t, proc, "original", "READY")
+	runtime.health("original", "READY")
@@ -263 +287 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stopRecoveryProcess(t, proc)
+	runtime.stop()
@@ -265,2 +289,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	cold := stand.boot("recovery-same-db.log")
-	stand.recoveryHealth(t, cold, "cold restart, same database", "CLOSED")
+	bootStand("recovery-same-db.log")
+	runtime.health("cold restart, same database", "CLOSED")
@@ -268 +292 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stopRecoveryProcess(t, cold)
+	runtime.stop()
@@ -270,0 +295 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+	queriesBefore := deliveryQueries.Load()
@@ -288,0 +314 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+	t.Logf("source safety: %+v", before)
@@ -307 +333,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	if after := restoredIdentity(t, target, ws, cid); before != after {
+	after := restoredIdentity(t, target, ws, cid)
+	if before != after {
@@ -317,0 +345,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+	t.Logf("restored safety: %+v", after)
+	t.Logf("restored intent: state=%s lease=%t attempts=%d expiry_unchanged=%t", held.state, held.lease, held.attempts, held.expiry.Equal(first.expiry))
@@ -325,2 +354,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	normal := stand.bootWith("recovery-normal-closed.log", "-config", stand.config)
-	stand.recoveryHealth(t, normal, "ordinary configuration without authoritative stand inputs", "CLOSED")
+	runtime.boot("recovery-normal-closed.log", "-config", stand.config)
+	runtime.health("ordinary configuration without authoritative stand inputs", "CLOSED")
@@ -328 +357 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stopRecoveryProcess(t, normal)
+	runtime.stop()
@@ -331 +360 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	restored := stand.boot("recovery-target.log")
+	bootStand("recovery-target.log")
@@ -333 +362 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stand.recoveryHealth(t, restored, "target with stale boot snapshot", "CLOSED")
+	runtime.health("target with stale boot snapshot", "CLOSED")
@@ -338 +367 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stand.recoveryHealth(t, restored, "target with wrong generation", "CLOSED")
+	runtime.health("target with wrong generation", "CLOSED")
@@ -340 +369 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stand.recoveryHealth(t, restored, "target with fresh boot and generation", "READY")
+	runtime.health("target with fresh boot and generation", "READY")
@@ -369 +398 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stopRecoveryProcess(t, restored)
+	runtime.stop()
@@ -383,2 +412,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	fault := stand.boot("recovery-missing-content.log")
-	stand.recoveryHealth(t, fault, "target, one missing detail", "READY")
+	bootStand("recovery-missing-content.log")
+	runtime.health("target, one missing detail", "READY")
@@ -388,0 +418,12 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
+	// Preserve fixture ownership so the container sees corrupt bytes, even when
+	// the test driver bootstraps PostgreSQL as root.
+	info, err := os.Stat(saved)
+	if err != nil {
+		t.Fatal(err)
+	}
+	owner := info.Sys().(*syscall.Stat_t)
+	if int(owner.Uid) != os.Geteuid() || int(owner.Gid) != os.Getegid() {
+		if err := os.Chown(activePath, int(owner.Uid), int(owner.Gid)); err != nil {
+			t.Fatal(err)
+		}
+	}
@@ -394 +435 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stopRecoveryProcess(t, fault)
+	runtime.stop()
@@ -419,2 +460,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	afterPurge := stand.boot("recovery-after-purge.log")
-	stand.recoveryHealth(t, afterPurge, "target after expiry cleanup", "READY")
+	bootStand("recovery-after-purge.log")
+	runtime.health("target after expiry cleanup", "READY")
@@ -426 +467 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	stopRecoveryProcess(t, afterPurge)
+	runtime.stop()
@@ -429 +470,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-		stand.deliveryCount.Load() != 1 || recoveredOutbox(t, target, ws, f.lineage) != held {
+		stand.deliveryCount.Load() != 1 || deliveryQueries.Load() != queriesBefore ||
+		recoveredOutbox(t, target, ws, f.lineage) != held {
@@ -432 +474,2 @@ func TestProcessingRecoveryOwnedStand(t *testing.T) {
-	t.Logf("RECOVERY PASS: live provider create calls=1, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=%d", time.Since(started).Milliseconds())
+	t.Logf("RECOVERY PASS: controlled Delivery create calls=%d, query calls before/after=%d/%d, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=%d",
+		stand.deliveryCount.Load(), queriesBefore, deliveryQueries.Load(), time.Since(started).Milliseconds())
diff --git a/services/processing/scripts/postgres/verify_image.py b/services/processing/scripts/postgres/verify_image.py
index 70f5d17d5233e6071630bf6fd301df0d08d7c102..d88a98361609374e979df2eac5ce22d18102c8c3 100644
--- a/services/processing/scripts/postgres/verify_image.py
+++ b/services/processing/scripts/postgres/verify_image.py
@@ -68,0 +69,2 @@ class ImageRun(Run):
+    task = "HIT-145"
+    test_summary = "CLOSED, forbidden Health, mTLS, READY, restart, durable idempotence"
@@ -74 +76 @@ class ImageRun(Run):
-        self.name = self.name.replace("hit76-", "hit145-", 1)
+        self.name = self.name.replace("hit76-", self.task.lower().replace("-", "") + "-", 1)
@@ -94 +96 @@ class ImageRun(Run):
-                self.docker("volume-create", "volume", "create", "--label", "hitpipe.task=HIT-145", volume)
+                self.docker("volume-create", "volume", "create", "--label", "hitpipe.task=" + self.task, volume)
@@ -100 +102 @@ class ImageRun(Run):
-            args[args.index("hitpipe.task=HIT-76")] = "hitpipe.task=HIT-145"
+            args[args.index("hitpipe.task=HIT-76")] = "hitpipe.task=" + self.task
@@ -152 +154 @@ class ImageRun(Run):
-                           "tests": "CLOSED, forbidden Health, mTLS, READY, restart, durable idempotence",
+                           "tests": self.test_summary,
@@ -157 +159 @@ class ImageRun(Run):
-    def await_file(self, name):
+    def await_file(self, name, status="status"):
@@ -163 +165 @@ class ImageRun(Run):
-            if name != "status":
+            if name != status:
@@ -165 +167 @@ class ImageRun(Run):
-                                   CONTROL + "/status", required=False)
+                                   CONTROL + "/" + status, required=False)
@@ -171,0 +174,2 @@ class ImageRun(Run):
+        if set(paths) not in ({"config"}, {"config", "stand"}):
+            raise ValueError("unexpected fixture arguments")
@@ -175,0 +180,3 @@ class ImageRun(Run):
+        args = ["-config", paths["config"]]
+        if "stand" in paths:
+            args += ["-stand-config", paths["stand"]]
@@ -177 +184 @@ class ImageRun(Run):
-                    "--pull=never", "--label", "hitpipe.task=HIT-145", "--network", "container:" + self.container,
+                    "--pull=never", "--label", "hitpipe.task=" + self.task, "--network", "container:" + self.container,
@@ -180 +187 @@ class ImageRun(Run):
-                    self.tag, "-config", paths["config"], "-stand-config", paths["stand"])
+                    self.loaded_id, *args)
@@ -197 +204 @@ class ImageRun(Run):
-    def stop_app(self):
+    def stop_app(self, kill=False):
@@ -199 +206,5 @@ class ImageRun(Run):
-        self.docker("processing-stop", "stop", "-t", "7", name)
+        if kill:
+            self.docker("processing-kill", "kill", "--signal", "KILL", name)
+            self.docker("processing-wait", "wait", name)
+        else:
+            self.docker("processing-stop", "stop", "-t", "7", name)
@@ -202,2 +213,3 @@ class ImageRun(Run):
-        if json.loads(state)["ExitCode"] != 0:
-            raise AssertionError("Processing did not exit normally")
+        result = json.loads(state)
+        if result["ExitCode"] != (137 if kill else 0) or result["OOMKilled"]:
+            raise AssertionError("unexpected Processing exit: " + state.decode())
diff --git a/services/processing/scripts/recovery/verify_oci_recovery.py b/services/processing/scripts/recovery/verify_oci_recovery.py
new file mode 100644
index 0000000000000000000000000000000000000000..a69fdb1a597a3b35cdaae46d4ba56c9a3bc157bd
--- /dev/null
+++ b/services/processing/scripts/recovery/verify_oci_recovery.py
@@ -0,0 +1,90 @@
+#!/usr/bin/env python3
+"""Run the existing image and recovery scenarios on one immutable TEST image."""
+import argparse
+import hashlib
+import json
+import pathlib
+import sys
+import tempfile
+
+sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / "postgres"))
+from verify_image import CONFIG, CONTENT, CONTROL, UID, ImageRun
+
+
+class RecoveryImageRun(ImageRun):
+    task = "HIT-155"
+    test_summary = "HIT-145 image restart and HIT-128 two-cluster recovery on the same inspected OCI image"
+
+    def __init__(self, output, go, fixture):
+        super().__init__(output, go, fixture)
+        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
+
+    def checks(self):
+        go = ["env", "GOTOOLCHAIN=local", "GOWORK=off", self.go, "-C", "services/processing"]
+        self.command("go-race", [*go, "test", "-race", "-count=1", "./..."])
+        self.command("go-vet", [*go, "vet", "./..."])
+        self.command("build-purge", ["env", "GOOS=linux", "GOARCH=" + self.identity["platform"].split("/")[1],
+                                    "CGO_ENABLED=0", *go, "build", "-trimpath", "-o",
+                                    str(self.fixture / "processing-purge"), "./cmd/processing-purge"])
+        super().checks()
+        self.check_recovery()
+
+    def check_recovery(self):
+        status = "recovery-status"
+        runner = (f"/test/storage.test -test.v -test.count=1 -test.timeout=110s "
+                  f"-test.run=^TestProcessingOCIRecoveryOwnedStand$ >{CONTROL}/recovery.stdout 2>{CONTROL}/recovery.stderr; "
+                  f'code=$?; printf "%s\\n" "$code" >{CONTROL}/{status}.tmp && mv {CONTROL}/{status}.tmp {CONTROL}/{status}')
+        self.docker("recovery-driver", "exec", "--detach", "--user", "0", "--env", "HIT78_ISOLATED_PG=1",
+                    "--env", "HIT128_RECOVERY_STAND=1", "--env", "HIT155_SOURCE_SHA=" + self.source,
+                    "--env", "HIT126_PURGE_BIN=/test/processing-purge", "--env", "TMPDIR=" + CONFIG + "/fixture",
+                    self.container, "sh", "-c", runner)
+        try:
+            for step in range(1, 13):
+                request = json.loads(self.await_file(f"recovery-{step}.json", status))
+                if set(request) != {"operation", "args"}:
+                    raise ValueError("unexpected recovery control fields")
+                args = request["args"]
+                if step % 2:
+                    if (request["operation"] != "start" or not isinstance(args, list)
+                            or len(args) not in (2, 4) or args[0] != "-config"
+                            or (len(args) == 4 and args[2] != "-stand-config")):
+                        raise ValueError("unexpected recovery start request")
+                    paths = {"config": args[1]}
+                    if len(args) == 4:
+                        paths["stand"] = args[3]
+                    # The Go recovery fixture owns the disposable cluster setup as
+                    # root. Make its private volumes usable by the actual OCI UID.
+                    self.docker("recovery-volume-ownership", "exec", "--user", "0", self.container,
+                                "chown", "-R", UID, CONFIG + "/fixture", CONTENT)
+                    self.start_app("recovery-" + str(step // 2 + 1), paths)
+                else:
+                    if request != {"operation": "kill", "args": None}:
+                        raise ValueError("unexpected recovery stop request")
+                    self.stop_app(kill=True)
+                self.docker("recovery-ack", "exec", self.container, "touch", f"{CONTROL}/recovery-ack-{step}")
+            result = self.await_file(status, status)
+            if result.strip() != b"0":
+                raise RuntimeError("OCI recovery fixture failed")
+        finally:
+            stdout = self.docker("recovery-stdout", "exec", self.container, "cat", CONTROL + "/recovery.stdout",
+                                 required=False)
+            self.docker("recovery-stderr", "exec", self.container, "cat", CONTROL + "/recovery.stderr", required=False)
+            if self.app_name:
+                self.docker("recovery-incomplete-logs", "logs", self.app_name, required=False)
+                self.docker("recovery-forced-cleanup", "rm", "-f", "-v", self.app_name, required=False)
+                self.app_name = None
+        if b"RECOVERY PASS:" not in stdout.stdout:
+            raise RuntimeError("OCI recovery fixture did not verify the restored state")
+
+
+if __name__ == "__main__":
+    parser = argparse.ArgumentParser(description=__doc__)
+    parser.add_argument("--go", required=True, type=pathlib.Path, help="absolute path to Go 1.27.2")
+    parser.add_argument("--output", required=True, type=pathlib.Path, help="new directory outside the checkout")
+    args = parser.parse_args()
+    if not args.go.is_absolute():
+        parser.error("--go must be absolute")
+    with tempfile.TemporaryDirectory(prefix="hit155-") as scratch:
+        fixture = pathlib.Path(scratch)
+        fixture.chmod(0o755)
+        RecoveryImageRun(args.output.resolve(), str(args.go), fixture).run()
```
