enum PlaybackFailureKind {
  ticketExpired,
  network,
  decoding,
  sourceUnavailable,
  unknown,
}

class PlaybackFailure {
  const PlaybackFailure({required this.kind, required this.message});

  final PlaybackFailureKind kind;
  final String message;

  static PlaybackFailure classify(String raw) {
    final message = raw.toLowerCase();
    if (message.contains('401') ||
        message.contains('403') ||
        message.contains('expired')) {
      return PlaybackFailure(
        kind: PlaybackFailureKind.ticketExpired,
        message: raw,
      );
    }
    if (message.contains('network') ||
        message.contains('timed out') ||
        message.contains('connection')) {
      return PlaybackFailure(kind: PlaybackFailureKind.network, message: raw);
    }
    if (message.contains('decode') ||
        message.contains('codec') ||
        message.contains('demux')) {
      return PlaybackFailure(kind: PlaybackFailureKind.decoding, message: raw);
    }
    if (message.contains('404') || message.contains('not found')) {
      return PlaybackFailure(
        kind: PlaybackFailureKind.sourceUnavailable,
        message: raw,
      );
    }
    return PlaybackFailure(kind: PlaybackFailureKind.unknown, message: raw);
  }
}
