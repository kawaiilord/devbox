import 'dart:math' as math;

import '../models.dart';
import '../player/media_player_kernel.dart';

enum AlignmentAction {
  reloadSource,
  switchEpisode,
  hardSeek,
  softSlow,
  softFast,
  aligned,
}

class AlignmentPlan {
  const AlignmentPlan({
    required this.action,
    required this.targetPosition,
    required this.rate,
    required this.playing,
  });

  final AlignmentAction action;
  final Duration targetPosition;
  final double rate;
  final bool playing;
}

class PlaybackSynchronizer {
  const PlaybackSynchronizer({
    this.softThreshold = const Duration(milliseconds: 300),
    this.hardSeekThreshold = const Duration(milliseconds: 1500),
  });

  final Duration softThreshold;
  final Duration hardSeekThreshold;

  AlignmentPlan plan({
    required PlaybackSnapshot snapshot,
    required Duration localPosition,
    required int localSourceVersion,
    required int localEpisode,
    required DateTime localNow,
    required Duration clockOffset,
  }) {
    final serverNow =
        localNow.millisecondsSinceEpoch + clockOffset.inMilliseconds;
    final elapsedMilliseconds = math.max(
      0,
      serverNow - snapshot.positionTimestamp,
    );
    final elapsedSeconds = snapshot.playing
        ? elapsedMilliseconds / 1000 * snapshot.speed
        : 0.0;
    final targetSeconds = math.max(0.0, snapshot.position + elapsedSeconds);
    final target = Duration(milliseconds: (targetSeconds * 1000).round());

    if (snapshot.sourceVersion != localSourceVersion) {
      return AlignmentPlan(
        action: AlignmentAction.reloadSource,
        targetPosition: target,
        rate: snapshot.speed,
        playing: snapshot.playing,
      );
    }
    if (snapshot.episode != localEpisode) {
      return AlignmentPlan(
        action: AlignmentAction.switchEpisode,
        targetPosition: target,
        rate: snapshot.speed,
        playing: snapshot.playing,
      );
    }

    final drift = localPosition - target;
    if (drift.abs() > hardSeekThreshold) {
      return AlignmentPlan(
        action: AlignmentAction.hardSeek,
        targetPosition: target,
        rate: snapshot.speed,
        playing: snapshot.playing,
      );
    }
    if (drift.abs() > softThreshold) {
      return AlignmentPlan(
        action: drift.isNegative
            ? AlignmentAction.softFast
            : AlignmentAction.softSlow,
        targetPosition: target,
        rate: snapshot.speed * (drift.isNegative ? 1.02 : 0.98),
        playing: snapshot.playing,
      );
    }
    return AlignmentPlan(
      action: AlignmentAction.aligned,
      targetPosition: target,
      rate: snapshot.speed,
      playing: snapshot.playing,
    );
  }

  Future<AlignmentPlan> apply({
    required PlaybackSnapshot snapshot,
    required MediaPlayerKernel player,
    required String sourceUrl,
    required Duration clockOffset,
  }) async {
    final alignment = plan(
      snapshot: snapshot,
      localPosition: player.position,
      localSourceVersion: player.sourceVersion,
      localEpisode: player.episode,
      localNow: DateTime.now(),
      clockOffset: clockOffset,
    );
    switch (alignment.action) {
      case AlignmentAction.reloadSource:
      case AlignmentAction.switchEpisode:
        await player.open(
          sourceUrl,
          version: snapshot.sourceVersion,
          episodeIndex: snapshot.episode,
          initialPosition: alignment.targetPosition,
        );
      case AlignmentAction.hardSeek:
        await player.seek(alignment.targetPosition);
      case AlignmentAction.softSlow:
      case AlignmentAction.softFast:
      case AlignmentAction.aligned:
        break;
    }
    await player.setRate(alignment.rate);
    if (alignment.playing && !player.playing) {
      await player.play();
    } else if (!alignment.playing && player.playing) {
      await player.pause();
    }
    return alignment;
  }
}
