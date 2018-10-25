# Makefile

# Output directory, all build products go under here.
OUTDIR:=artifacts

IMAGE_NAME:=kojustin-trials:latest

# Default rule builds everything. In addition also re-builds the Docker image.
.PHONY: all
all: $(OUTDIR)/image_name.txt

.PHONY: clean
clean:
	rm -rf $(OUTDIR)

# Build intermediate directories
$(OUTDIR) $(OUTDIR)/containerize:
	mkdir -p $@

# Compile the binary, place it into the output directory.
$(OUTDIR)/containerize/trial: *.go | $(OUTDIR)/containerize
	go build -o $@

$(OUTDIR)/containerize/Dockerfile: Dockerfile.template
	cp $< $@

# This target builds the final deployable Docker image. We use a text file to
# represent the Docker image because images aren't filesystem objects and Make
# *only* knows about filesystem objects.
$(OUTDIR)/image_name.txt: $(OUTDIR)/containerize/trial $(OUTDIR)/containerize/Dockerfile
	docker build --tag $(IMAGE_NAME) $(OUTDIR)/containerize
	echo $(IMAGE_NAME) > $@
