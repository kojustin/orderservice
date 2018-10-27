# Makefile

# Output directory, all build products go under here.
OUTDIR:=artifacts

SHELL:=/bin/bash

IMAGE_NAME:=kojustin-orderservice:latest

# Default rule builds everything. In addition also re-builds the Docker image.
.PHONY: all
all: $(OUTDIR)/image_name.txt

.PHONY: clean
clean:
	rm -rf $(OUTDIR)

.PHONY: test
test: *.go Makefile
	go vet
	if [[ -n "$$(goimports -d .)" ]]; then echo "imports wrong!"; exit 1; fi
	go test -v

# Build intermediate directories
$(OUTDIR) $(OUTDIR)/containerize:
	mkdir -p $@

# Compile the binary, place it into the output directory.
$(OUTDIR)/containerize/orderservice: *.go Makefile | $(OUTDIR)/containerize
	CGO_ENABLED=1 GOOS=linux go build -o $@

$(OUTDIR)/containerize/Dockerfile: Dockerfile.template | $(OUTDIR)/containerize
	cp $< $@

# This target builds the final deployable Docker image. We use a text file to
# represent the Docker image because images aren't filesystem objects and Make
# *only* knows about filesystem objects.
$(OUTDIR)/image_name.txt: $(OUTDIR)/containerize/orderservice $(OUTDIR)/containerize/Dockerfile | $(OUTDIR)
	docker build --tag $(IMAGE_NAME) $(OUTDIR)/containerize
	echo $(IMAGE_NAME) > $@
