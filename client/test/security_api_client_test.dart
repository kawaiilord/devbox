import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sameframe_client/src/api/api_client.dart';
import 'package:sameframe_client/src/models.dart';

void main() {
  test('MFA confirmation keeps recovery codes and replaces the application session', () async {
    final session = _session('old');
    var saved = '';
    final client = MockClient((request) async {
      if (request.url.path.endsWith('/totp/confirm')) {
        expect(request.headers['Authorization'], 'Bearer old-access');
        expect(jsonDecode(request.body), {
          'challenge': 'single-use-challenge',
          'code': '123456',
        });
        return _response({
          'session': _session('secured').toJson(),
          'recovery_codes': ['ONE-TIME-CODE'],
        });
      }
      expect(request.headers['Authorization'], 'Bearer secured-access');
      return _response({'totp_enabled': true});
    });
    final api = ApiClient(client: client, baseUrl: 'https://account.test')
      ..onSessionUpdated = (value) async {
        saved = value.refreshToken;
      };
    final codes = await api.confirmTOTP(
      session,
      'single-use-challenge',
      '123456',
    );
    expect(codes, ['ONE-TIME-CODE']);
    expect(saved, 'secured-refresh');
    expect((await api.accountSecurity(session))['totp_enabled'], true);
    api.close();
  });

  test(
    'a consumed security ceremony is not replayed on an authentication error',
    () async {
      final paths = <String>[];
      final client = MockClient((request) async {
        paths.add(request.url.path);
        return http.Response(
          jsonEncode({'code': 401, 'msg': 'ceremony expired'}),
          401,
        );
      });
      final api = ApiClient(client: client, baseUrl: 'https://account.test');
      await expectLater(
        api.finishPasskey(_session('old'), 'ceremony', {'id': 'credential'}),
        throwsA(isA<ApiException>()),
      );
      expect(paths, ['/api/v1/account/passkeys/finish']);
      api.close();
    },
  );

  test(
    'a refresh already in flight cannot overwrite a newer security session',
    () async {
      final refreshStarted = Completer<void>();
      final refreshResponse = Completer<http.Response>();
      final persisted = <String>[];
      final session = _session('old');
      final client = MockClient((request) async {
        if (request.url.path == '/api/v1/auth/refresh') {
          refreshStarted.complete();
          return refreshResponse.future;
        }
        if (request.url.path.endsWith('/totp/disable')) {
          return _response(_session('secured').toJson());
        }
        if (request.headers['Authorization'] == 'Bearer old-access') {
          return http.Response(
            jsonEncode({'code': 401, 'msg': 'expired'}),
            401,
          );
        }
        expect(request.headers['Authorization'], 'Bearer secured-access');
        return _response({'devices': <Object>[]});
      });
      final api = ApiClient(client: client, baseUrl: 'https://account.test')
        ..onSessionUpdated = (value) async {
          persisted.add(value.accessToken);
        };
      final devices = api.listDevices(session);
      await refreshStarted.future;
      await api.disableTOTP(session, {
        'password': 'password',
        'code': 'recovery',
      });
      refreshResponse.complete(_response(_session('stale').toJson()));
      await devices;
      expect(session.accessToken, 'secured-access');
      expect(persisted, ['secured-access']);
      api.close();
    },
  );
}

Session _session(String prefix) => Session(
  accessToken: '$prefix-access',
  refreshToken: '$prefix-refresh',
  tokenType: 'bearer',
  expiresIn: 900,
  user: const AppUser(
    id: 'owner',
    displayName: 'Owner',
    email: 'owner@account.test',
    emailVerified: true,
  ),
);
http.Response _response(Map<String, dynamic> data) =>
    http.Response(jsonEncode({'code': 0, 'data': data}), 200);
