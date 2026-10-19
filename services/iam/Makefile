.PHONY: check dev release smoke
check:
	python3 ../../tools/pr12/workspace.py check iam --require
dev:
	python3 ../../tools/pr12/workspace.py build iam --mode dev --require
release:
	python3 ../../tools/pr12/workspace.py build iam --mode release --require
smoke:
	python3 ../../tools/pr12/workspace.py smoke iam --require
