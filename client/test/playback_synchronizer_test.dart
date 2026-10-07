import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/models.dart';
import 'package:sameframe_client/src/sync/playback_synchronizer.dart';

void main() {
  const synchronizer = PlaybackSynchronizer();
  final now = DateTime.fromMillisecondsSinceEpoch(10000);

  PlaybackSnapshot snapshot({double position = 8, bool playing = true}) =>
      PlaybackSnapshot(
        position: position,
        playing: playing,
        speed: 1,
        episode: 0,
        positionTimestamp: 9000,
        sourceVersion: 1,
      );

  test('hard seeks when drift exceeds threshold', () {
    final plan = synchronizer.plan(
      snapshot: snapshot(),
      localPosition: const Duration(seconds: 5),
      localSourceVersion: 1,
      localEpisode: 0,
      localNow: now,
      clockOffset: Duration.zero,
    );
    expect(plan.action, AlignmentAction.hardSeek);
    expect(plan.targetPosition, const Duration(seconds: 9));
  });

  test('softly speeds up a client that is slightly behind', () {
    final plan = synchronizer.plan(
      snapshot: snapshot(),
      localPosition: const Duration(milliseconds: 8500),
      localSourceVersion: 1,
      localEpisode: 0,
      localNow: now,
      clockOffset: Duration.zero,
    );
    expect(plan.action, AlignmentAction.softFast);
    expect(plan.rate, closeTo(1.02, 0.001));
  });

  test('clock offset participates in target projection', () {
    final plan = synchronizer.plan(
      snapshot: snapshot(position: 8),
      localPosition: const Duration(seconds: 10),
      localSourceVersion: 1,
      localEpisode: 0,
      localNow: now,
      clockOffset: const Duration(seconds: 1),
    );
    expect(plan.targetPosition, const Duration(seconds: 10));
    expect(plan.action, AlignmentAction.aligned);
  });
}
