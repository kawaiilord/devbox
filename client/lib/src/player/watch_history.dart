import 'dart:convert';

import 'package:crypto/crypto.dart';
import 'package:shared_preferences/shared_preferences.dart';

class WatchHistory {
  WatchHistory(String user, String target)
    : key = 'watch.v1.${sha256.convert(utf8.encode('$user|$target'))}';
  final String key;
  SharedPreferences? _preferences;
  Future<Map<String, dynamic>?> read() async {
    try {
      _preferences = await SharedPreferences.getInstance();
      final raw = _preferences!.getString(key);
      return raw == null ? null : jsonDecode(raw) as Map<String, dynamic>;
    } catch (_) {
      // Storage can be disabled; watching must still work.
      return null;
    }
  }

  Future<void> save(Map<String, dynamic> value) async {
    try {
      final prefs = _preferences ??= await SharedPreferences.getInstance();
      await prefs.setString(
        key,
        jsonEncode({
          ...value,
          'updated': DateTime.now().millisecondsSinceEpoch,
        }),
      );
      final keys = prefs
          .getKeys()
          .where((k) => k.startsWith('watch.v1.'))
          .toList();
      if (keys.length > 120) {
        keys.sort((a, b) {
          int date(String k) {
            try {
              return (jsonDecode(prefs.getString(k)!)['updated'] as num?)
                      ?.toInt() ??
                  0;
            } catch (_) {
              return 0;
            }
          }

          return date(a).compareTo(date(b));
        });
        for (final old in keys.take(keys.length - 120)) {
          await prefs.remove(old);
        }
      }
    } catch (_) {
      /* Private browsing or full local storage. */
    }
  }
}
