# Run from services/processing with Go 1.27.2 on PATH.
.PHONY: check smoke dev release
check:
	python3 ../../tools/pr12/workspace.py check processing --require
smoke:
	python3 ../../tools/pr12/workspace.py smoke processing --require
dev:
	python3 ../../tools/pr12/workspace.py build processing --mode dev --require
release:
	python3 ../../tools/pr12/workspace.py build processing --mode release --require
