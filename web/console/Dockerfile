FROM node:26.12.1-alpine3.24@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS node-toolchain
RUN npm install --global npm@12.2.1

FROM node-toolchain AS deps
WORKDIR /app
COPY package.json package-lock.json .npmrc ./
COPY scripts ./scripts
RUN npm ci

FROM node-toolchain AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:26.12.1-alpine3.24@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
# The standalone server only needs the Node runtime. Do not retain package-manager
# toolchains (or their dependency graphs) in the production image.
RUN rm -rf /usr/local/lib/node_modules/npm /opt/yarn-v* \
    && rm -f /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/yarn /usr/local/bin/yarnpkg
COPY --from=builder --chown=node:node /app/.next/standalone ./
COPY --from=builder --chown=node:node /app/.next/static ./.next/static
COPY --from=builder --chown=node:node /app/public ./public
COPY --from=builder --chown=node:node /app/scripts/console-runtime.mjs /app/scripts/runtime-proxy.mjs ./scripts/
USER node
EXPOSE 3000
CMD ["node", "scripts/console-runtime.mjs", "server.js"]
