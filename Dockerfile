FROM alpine:latest

RUN apk add --update go git

ENV GOPATH /go

ADD . /go/src/github.com/janeprather/xapi_exporter
RUN go get -d github.com/janeprather/xapi_exporter
RUN go build -o /bin/xapi_exporter github.com/janeprather/xapi_exporter
RUN apk del go git
RUN rm -rf /go

EXPOSE 9290

VOLUME ["/xapi_exporter"]

CMD /bin/xapi_exporter -config /xapi_exporter/config.yml
