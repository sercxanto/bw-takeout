FROM node:24-bookworm-slim@sha256:dc26ae92c9fb15cf6b1d63e20f0944b2efe0f115b20e00be48fb23fceb061c54 AS node
FROM golang:1.27.3-bookworm@sha256:dc0963f32053e4bc451d24d87e3892d42d01021b61f093e98eb95c0352bb84b0
COPY --from=node /usr/local/bin/node /usr/local/bin/
COPY --from=node /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm
WORKDIR /opt/bootstrap
COPY package*.json ./
RUN npm ci --no-audit --no-fund && ln -s /opt/bootstrap/node_modules/.bin/openspec /usr/local/bin/openspec && ln -s /opt/bootstrap/node_modules/.bin/bw /usr/local/bin/bw
RUN GOBIN=/usr/local/bin go install filippo.io/age/cmd/age@v1.2.1 && GOBIN=/usr/local/bin go install filippo.io/age/cmd/age-keygen@v1.2.1
RUN useradd -m -s /bin/bash vscode && chown -R vscode:vscode /go
USER vscode
ENV OPENSPEC_TELEMETRY=0
WORKDIR /home/vscode
RUN go version && openspec --version && bw --version && age --version
