{{flutter_js}}
{{flutter_build_config}}

// Ship the matching renderer with the app so startup does not depend on a
// third-party CDN being reachable from the viewer's network.
_flutter.loader.load({
  config: {canvasKitBaseUrl: 'canvaskit/'},
  serviceWorkerSettings: {
    serviceWorkerVersion: {{flutter_service_worker_version}}
  }
});
