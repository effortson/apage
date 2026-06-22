# Multi-stage build for any apage Go service. Pass SERVICE=api|gateway|worker|agent.
# Spec §22.3: each service is an independent container/process.
FROM golang:1.23-alpine AS build
ARG SERVICE=api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/apage-${SERVICE} ./cmd/${SERVICE}

FROM alpine:3.20
ARG SERVICE=api
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 apage
USER apage
COPY --from=build /out/apage-${SERVICE} /usr/local/bin/apage-svc
# Logs to stdout/stderr (spec §22.3); config via env (spec §33).
ENTRYPOINT ["/usr/local/bin/apage-svc"]
