import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

import 'local_media_proxy.dart';
import 'adaptive_media.dart';

class MediaPlayerKernel {
  MediaPlayerKernel()
    : player = Player(),
      generation = 0,
      sourceVersion = 0,
      episode = 0 {
    videoController = VideoController(player);
    _proxy = LocalMediaProxy.create();
    _errorSubscription = player.stream.error.listen(_errors.add);
  }

  final Player player;
  late final VideoController videoController;
  late final Future<LocalMediaProxy> _proxy;
  int generation;
  int sourceVersion;
  int episode;
  final _errors = StreamController<String>.broadcast();
  StreamSubscription<String>? _errorSubscription;

  Duration get position => player.state.position;
  Duration get duration => player.state.duration;
  bool get playing => player.state.playing;

  Stream<Duration> get positionStream => player.stream.position;
  Stream<Duration> get durationStream => player.stream.duration;
  Stream<bool> get playingStream => player.stream.playing;
  Stream<String> get errorStream => _errors.stream;

  Future<void> open(
    String source, {
    required int version,
    required int episodeIndex,
    Duration initialPosition = Duration.zero,
    String? cacheIdentity,
    bool live = false,
  }) async {
    final operationGeneration = ++generation;
    sourceVersion = version;
    episode = episodeIndex;
    final proxy = await _proxy;
    final adaptive = RegExp(
      r'\.(m3u8|mpd|flv)([?#]|$)',
      caseSensitive: false,
    ).hasMatch(source);
    final playbackUri = adaptive
        ? Uri.parse(source)
        : await proxy.prepare(Uri.parse(source), cacheIdentity: cacheIdentity);
    final handle = kIsWeb ? await player.handle : 0;
    if (kIsWeb) {
      await prepareAdaptive(handle, playbackUri.toString(), live, (message) {
        if (operationGeneration == generation && !_errors.isClosed) {
          _errors.add(message);
        }
      });
    }
    await player.open(Media(playbackUri.toString()), play: false);
    if (kIsWeb) {
      await readyAdaptive(handle);
    }
    if (operationGeneration != generation) return;
    if (initialPosition > Duration.zero) {
      await player.seek(initialPosition);
    }
  }

  Future<void> play() => player.play();
  Future<void> pause() => player.pause();
  Future<void> seek(Duration position) => player.seek(position);
  Future<void> setRate(double rate) => player.setRate(rate);
  Future<void> loadSubtitle(String uri, {String? title}) =>
      player.setSubtitleTrack(SubtitleTrack.uri(uri, title: title));
  Future<void> disableSubtitles() =>
      player.setSubtitleTrack(SubtitleTrack.no());

  Future<void> dispose() async {
    generation++;
    if (kIsWeb) {
      await disposeAdaptive(await player.handle);
    }
    await _errorSubscription?.cancel();
    await player.dispose();
    await (await _proxy).close();
    await _errors.close();
  }
}
