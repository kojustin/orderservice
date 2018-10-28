# Makefile
#
# Build things.

# Output directory, all build products go under here.
OUTDIR:=artifacts

# Make sure to use bash
SHELL:=/bin/bash

# Docker container name
IMAGE_NAME:=kojustin-orderservice:latest

# Default rule builds everything. In addition also re-builds the Docker image.
.PHONY: all
all: $(OUTDIR)/image_name.txt

.PHONY: clean
clean:
	rm -rf $(OUTDIR)

# This is the quick test. It runs some Go linting tools and then runs the unit
# tests.
.PHONY: test
test: *.go Makefile
	go vet
	if [[ -n "$$(gofmt -d .)" ]]; then echo "ERROR! Needs gofmt!"; exit 1; fi
	if [[ -n "$$(goimports -d .)" ]]; then echo "ERROR! Needs goimports!"; exit 1; fi
	go test -v

# Build intermediate directories
$(OUTDIR) $(OUTDIR)/svc:
	mkdir -p $@

# Initialize database
$(OUTDIR)/orders.db: schema.sql | $(OUTDIR)
	sqlite3 $@ < schema.sql

# Compile the binary, place it into the output directory.
$(OUTDIR)/svc/orderservice: *.go Makefile | $(OUTDIR)/svc
	CGO_ENABLED=1 GOOS=linux go build -o $@

$(OUTDIR)/svc/Dockerfile: Dockerfile.template | $(OUTDIR)/svc
	cp $< $@

# This target builds the final deployable Docker image. We use a text file to
# represent the Docker image because images aren't filesystem objects and Make
# *only* knows about filesystem objects.
$(OUTDIR)/image_name.txt: $(OUTDIR)/svc/orderservice $(OUTDIR)/svc/Dockerfile | $(OUTDIR)
	docker build --tag $(IMAGE_NAME) $(OUTDIR)/svc
	echo $(IMAGE_NAME) > $@
