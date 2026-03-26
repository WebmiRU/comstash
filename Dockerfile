FROM golang:1.21-alpine AS builder
# Install SSL certificates
RUN apk add --no-available --no-cache ca-certificates
WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -tags goexperiment.jsonv2 -o /app .


FROM scratch
COPY --from=builder /app /app
# COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ # for HTTPS requests (if needed)
ENTRYPOINT ["/app"]