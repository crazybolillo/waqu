FROM golang:1.23-alpine

WORKDIR /app

COPY . .

ENV CGO_ENABLED=0
RUN go build -o /bin/waqu ./main.go

FROM scratch

COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=0 /bin/waqu /bin/waqu

EXPOSE 8080

ENTRYPOINT ["waqu"]
