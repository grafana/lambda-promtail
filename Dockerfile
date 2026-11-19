FROM golang:1.26-alpine@sha256:ecb67f987aa1a57794c91d19ade7ae0209f2f0f5496aaef9e6ae85ddf92d02ab AS build-image

COPY . /src/lambda-promtail
WORKDIR /src/lambda-promtail

RUN go version

RUN apk update && apk upgrade && \
    apk add --no-cache bash git
RUN go version

RUN ls -al
RUN go mod download
RUN go build -o /main -tags lambda.norpc -ldflags="-s -w" pkg/*.go
# copy artifacts to a clean image
FROM public.ecr.aws/lambda/provided:al2023.2026.02.27.21@sha256:9ee3f032d4063febdf42a2a1f15149ce8358130753e48fcaf86eae731fab38ff
RUN dnf -y update && \
    dnf clean all
COPY --from=build-image /main /main
ENTRYPOINT [ "/main" ]
