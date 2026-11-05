SNOWHIVE ?= /home/prince/Desktop/Others

.EXPORT_ALL_VARIABLES:

run:
	@go run main.go $(ARGS)

run-download:
	go run main.go --mode download

run-steal:
	go run main.go --mode steal

run-validate:
	go run main.go --mode validate

mount-partition:
	sudo mount -a
