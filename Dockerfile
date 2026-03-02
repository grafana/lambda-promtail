FROM golang:1.26-alpine@sha256:d4c4845f5d60c6a974c6000ce58ae079328d03ab7f721a0734277e69905473e5 AS build-image

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
