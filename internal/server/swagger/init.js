window.ui = SwaggerUIBundle({
  url: './openapi.json', dom_id: '#swagger-ui', deepLinking: true,
  persistAuthorization: false, validatorUrl: null, queryConfigEnabled: false,
  supportedSubmitMethods: ['get', 'post'],
  plugins: [() => ({statePlugins: {spec: {wrapSelectors: {allowTryItOutFor: (original) => (state, path, method) =>
    path === '/v1/audio/stream' ? false : original(state, path, method)}}}})]
});
