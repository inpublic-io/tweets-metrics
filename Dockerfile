FROM golang:1.17-alpine as build

ADD . /go/src/github.com/inpublic-io/tweets-metrics

WORKDIR /go/src/github.com/inpublic-io/tweets-metrics

RUN go build -o "service" -tags musl ./

FROM alpine:3

LABEL org.opencontainers.image.source https://github.com/inpublic-io/tweets-metrics

RUN apk update \
	&& apk -U upgrade \
	&& apk add --no-cache ca-certificates bash \
	&& update-ca-certificates --fresh \
	&& rm -rf /var/cache/apk/*

COPY --from=build /go/src/github.com/inpublic-io/tweets-metrics/service /usr/local/bin/
RUN chmod +x /usr/local/bin/service

ENTRYPOINT [ "/usr/local/bin/service" ]