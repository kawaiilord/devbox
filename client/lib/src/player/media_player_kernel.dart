import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

import 'local_media_proxy.dart';

class MediaPlayerKernel {
  MediaPlayerKernel()
    : player = Player(),
      generation = 0,
      sourceVersion = 0,
      episode = 0 {
    videoController = VideoController(player);
    _proxy = LocalMediaProxy.create();
  }

  final Player player;
  late final VideoController videoController;
  late final Future<LocalMediaProxy> _proxy;
  int generation;
  int sourceVersion;
  int episode;

  Duration get position => player.state.position;
  Duration get duration => player.state.duration;
  bool get playing => player.state.playing;

  Stream<Duration> get positionStream => player.stream.position;
  Stream<Duration> get durationStream => player.stream.duration;
  Stream<bool> get playingStream => player.stream.playing;

  Future<void> open(
    String source, {
    required int version,
    required int episodeIndex,
    Duration initialPosition = Duration.zero,
    String? cacheIdentity,
  }) async {
    final operationGeneration = ++generation;
    sourceVersion = version;
    episode = episodeIndex;
    final proxy = await _proxy;
    final playbackUri = await proxy.prepare(
      Uri.parse(source),
      cacheIdentity: cacheIdentity,
    );
    await player.open(Media(playbackUri.toString()), play: false);
    if (operationGeneration != generation) return;
    if (initialPosition > Duration.zero) {
      await player.seek(initialPosition);
    }
  }

  Future<void> play() => player.play();
  Future<void> pause() => player.pause();
  Future<void> seek(Duration position) => player.seek(position);
  Future<void> setRate(double rate) => player.setRate(rate);

  Future<void> dispose() async {
    generation++;
    await player.dispose();
    await (await _proxy).close();
  }
}
