import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/sync/buffering_gate.dart';

void main() {
  test('owner buffering freezes actual position and resumes after buffered pause acknowledgement', () {
    final gate = BufferingGate();
    final now = DateTime.utc(2026, 10, 15);
    List<Map<String, dynamic>> observe(
      int ms, {
      bool ready = false,
      bool playing = true,
      int version = 1,
    }) => gate.observe(
      sourceVersion: version,
      playing: playing,
      ready: ready,
      position: 12.5,
      now: now.add(Duration(milliseconds: ms)),
    );
    expect(observe(0), isEmpty);
    expect(observe(500), isEmpty);
    expect(observe(1100), [
      {'action': 'pause'},
      {'action': 'seek', 'position': 12.5},
    ]);
    expect(observe(1500), isEmpty);
    expect(observe(2000, ready: true), isEmpty);
    expect(observe(2500, ready: true, playing: false), [
      {'action': 'play'},
    ]);
    expect(observe(3000, ready: true), isEmpty);
  });
  test('manual controls and source switches cancel pending auto resume', () {
    final gate = BufferingGate();
    final now = DateTime.utc(2026, 10, 15);
    for (final version in [1, 2]) {
      gate.observe(
        sourceVersion: version,
        playing: true,
        ready: false,
        position: 0,
        now: now,
      );
      gate.observe(
        sourceVersion: version,
        playing: true,
        ready: false,
        position: 0,
        now: now.add(const Duration(seconds: 2)),
      );
      if (version == 1) gate.reset();
      final result = gate.observe(
        sourceVersion: version == 1 ? 1 : 3,
        playing: false,
        ready: true,
        position: 0,
        now: now.add(const Duration(seconds: 3)),
      );
      expect(result, isEmpty);
    }
  });
}
