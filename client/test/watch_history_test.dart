import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:sameframe_client/src/player/watch_history.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test('watch progress persists and remains isolated between accounts and versions', () async {
    SharedPreferences.setMockInitialValues({});
    final history = WatchHistory('alice', 'movie|2026|电影');
    await history.save({'episode_name': '第2集', 'position': 92.5, 'rate': 1.5});
    expect(
      (await WatchHistory('alice', 'movie|2026|电影').read())?['position'],
      92.5,
    );
    expect(await WatchHistory('bob', 'movie|2026|电影').read(), isNull);
    expect(await WatchHistory('alice', 'movie|1999|电影').read(), isNull);
  });
}
