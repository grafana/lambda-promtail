FROM golang:1.27-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build-image

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
