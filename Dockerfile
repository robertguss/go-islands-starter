FROM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS generated
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go tool sqlc generate && go tool tygo generate && go tool shadcn-templ bundle && go tool templ generate

FROM node:24.19.0-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS frontend
WORKDIR /src
RUN npm install --global npm@11.17.0 --ignore-scripts
COPY web/package.json web/package-lock.json web/.npmrc ./web/
RUN cd web && npm ci --ignore-scripts
COPY --from=generated /src/web ./web
COPY --from=generated /src/components ./components
COPY --from=generated /src/templates ./templates
RUN cd web && npm run check && npm run build

FROM generated AS build
COPY --from=frontend /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/migrate ./cmd/migrate && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/admin ./cmd/admin

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=build /out/ /
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]
