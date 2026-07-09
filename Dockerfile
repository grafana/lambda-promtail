FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build-image

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
