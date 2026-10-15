Future<void> prepareAdaptive(
  int id,
  String url,
  bool live,
  void Function(String) onError,
) async {}
Future<void> readyAdaptive(int id) async {}
Future<void> disposeAdaptive(int id) async {}
Future<bool> adaptiveBuffered(int id) async => true;
Future<Map<String, dynamic>> adaptiveInfo(int id) async => {};
Future<void> adaptiveAction(int id, String name, [String value = '']) async {}
