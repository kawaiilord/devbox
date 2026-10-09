import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/player/playback_failure.dart';

void main() {
  test('classifies actionable playback failures', () {
    expect(
      PlaybackFailure.classify('HTTP 403 expired').kind,
      PlaybackFailureKind.ticketExpired,
    );
    expect(
      PlaybackFailure.classify('network connection timed out').kind,
      PlaybackFailureKind.network,
    );
    expect(
      PlaybackFailure.classify('codec decode failed').kind,
      PlaybackFailureKind.decoding,
    );
    expect(
      PlaybackFailure.classify('HTTP 404 not found').kind,
      PlaybackFailureKind.sourceUnavailable,
    );
  });
}
