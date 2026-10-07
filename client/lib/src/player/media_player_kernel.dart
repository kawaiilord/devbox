import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

class MediaPlayerKernel {
  MediaPlayerKernel()
    : player = Player(),
      generation = 0,
      sourceVersion = 0,
      episode = 0 {
    videoController = VideoController(player);
  }

  final Player player;
  late final VideoController videoController;
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
  }) async {
    final operationGeneration = ++generation;
    sourceVersion = version;
    episode = episodeIndex;
    await player.open(Media(source), play: false);
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
  }
}
