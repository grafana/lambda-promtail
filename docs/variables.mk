# List of projects to provide to the make-docs script.
# Format is PROJECT[:[VERSION][:[REPOSITORY][:[DIRECTORY]]]]
# It requires that you have a Loki repository checked out into the same directory as the one containing this repository.
PROJECTS := loki::$(notdir $(basename $(shell git rev-parse --show-toplevel)../loki)) \
	arbitrary:$(shell git rev-parse --show-toplevel)/docs/sources:/hugo/content/docs/loki/latest/send-data/lambda-promtail
