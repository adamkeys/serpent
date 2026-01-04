.PHONY: run

run: .pkg/.installed
	PYTHONPATH=./.pkg go run .

.pkg/.installed: requirements.txt
	@python3 -m pip install --upgrade --target .pkg -r requirements.txt
	@touch $@