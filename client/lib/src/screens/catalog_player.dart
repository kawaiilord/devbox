import 'dart:async';
import 'dart:math' as math;

import 'package:file_selector/file_selector.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../player/adaptive_media.dart';
import '../player/media_player_kernel.dart';

class CatalogPlayer extends StatefulWidget {
  const CatalogPlayer({
    super.key,
    required this.api,
    required this.session,
    required this.selection,
    required this.identity,
    this.onNext,
    this.onPrevious,
    this.onFailure,
    this.onProgress,
    this.startPosition = 0,
    this.startPlaying = true,
    this.startRate = 1,
    this.skipIntro = 0,
    this.skipOutro = 0,
    this.height = 480,
    this.autoAdvance = true,
  });
  final ApiClient api;
  final Session session;
  final Map<String, dynamic> selection;
  final String identity;
  final VoidCallback? onNext, onPrevious, onFailure;
  final void Function(
    String identity,
    double position,
    double duration,
    double rate,
  )?
  onProgress;
  final double startPosition, startRate, height;
  final bool startPlaying, autoAdvance;
  final int skipIntro, skipOutro;
  @override
  State<CatalogPlayer> createState() => CatalogPlayerState();
}

class CatalogPlayerState extends State<CatalogPlayer> {
  MediaPlayerKernel? _kernel;
  Timer? _renew;
  final List<StreamSubscription<dynamic>> _subscriptions = [];
  bool _loading = true, _failed = false, _completed = false;
  String? _error, _subtitle;
  int _request = 0, _lastReported = -1;
  double _position = 0, _duration = 0, _rate = 1, _volume = 80;
  bool _playing = true;
  String _quality = 'auto';
  Map<String, dynamic> _info = {};
  BoxFit _fit = BoxFit.contain;
  double get positionSeconds => _kernel == null || _loading
      ? _position
      : _kernel!.position.inMilliseconds / 1000;
  bool get playing => _kernel == null || _loading ? _playing : _kernel!.playing;
  double get rate => _rate;

  Future<void> pause() async {
    _playing = false;
    await _kernel?.pause();
    _report();
  }

  Future<void> resume() async {
    _playing = true;
    await _kernel?.play();
  }

  void _report() =>
      widget.onProgress?.call(widget.identity, _position, _duration, _rate);

  @override
  void initState() {
    super.initState();
    unawaited(_open());
  }

  @override
  void didUpdateWidget(CatalogPlayer oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.identity != widget.identity) unawaited(_open());
  }

  @override
  void dispose() {
    _report();
    _request++;
    _renew?.cancel();
    for (final subscription in _subscriptions) {
      unawaited(subscription.cancel());
    }
    unawaited(_kernel?.dispose());
    super.dispose();
  }

  void _fail(int request, Object error) {
    if (!mounted || request != _request || _failed) return;
    setState(() {
      _failed = true;
      _loading = false;
      _error = error is ApiException ? error.message : '这条线路暂时无法播放，请重试或切换其他来源。';
    });
    widget.onFailure?.call();
  }

  Future<void> _open({bool preserve = false}) async {
    final request = ++_request;
    _renew?.cancel();
    final previous = _kernel;
    _volume = previous?.player.state.volume ?? _volume;
    final subscriptions = [..._subscriptions];
    _subscriptions.clear();
    final position = preserve ? positionSeconds : widget.startPosition;
    final play = preserve ? playing : widget.startPlaying;
    final rate = preserve ? _rate : widget.startRate;
    setState(() {
      _kernel = null;
      _position = position;
      _playing = play;
      _rate = rate;
      _duration = 0;
      _loading = true;
      _error = null;
      _failed = false;
      _completed = false;
      _lastReported = -1;
      _quality = 'auto';
      _info = {};
    });
    for (final subscription in subscriptions) {
      await subscription.cancel();
    }
    await previous?.dispose();
    if (!mounted || request != _request) return;
    try {
      final ticket = await widget.api.watchCatalog(
        widget.session,
        widget.selection,
      );
      if (!mounted || request != _request) return;
      final kernel = MediaPlayerKernel();
      setState(() => _kernel = kernel);
      _subscriptions.add(
        kernel.errorStream.listen((message) => _fail(request, message)),
      );
      void next() {
        if (_completed ||
            _loading ||
            _failed ||
            !mounted ||
            request != _request) {
          return;
        }
        _completed = true;
        if (widget.autoAdvance) widget.onNext?.call();
      }

      _subscriptions.add(
        kernel.player.stream.completed.listen((done) {
          if (done) next();
        }),
      );
      _subscriptions.add(
        kernel.positionStream.listen((value) {
          if (!mounted || request != _request || _loading) return;
          _position = value.inMilliseconds / 1000;
          if (widget.skipOutro > 0 &&
              _duration > widget.skipOutro + widget.skipIntro + 10 &&
              _position >= _duration - widget.skipOutro &&
              kernel.playing) {
            next();
          }
          if (value.inSeconds ~/ 5 != _lastReported) {
            _lastReported = value.inSeconds ~/ 5;
            _report();
          }
        }),
      );
      _subscriptions.add(
        kernel.durationStream.listen((value) {
          if (mounted && request == _request) {
            _duration = value.inMilliseconds / 1000;
          }
        }),
      );
      _subscriptions.add(
        kernel.playingStream.listen((value) {
          if (mounted && request == _request && !_loading) {
            _playing = value;
            if (!value) _report();
          }
        }),
      );
      await kernel.open(ticket.url, version: request, episodeIndex: 0);
      if (!mounted || request != _request) return;
      final duration = kernel.duration.inMilliseconds / 1000;
      final desired = position > 0 ? position : widget.skipIntro.toDouble();
      if (desired > 0) {
        await kernel.seek(
          Duration(
            milliseconds:
                ((duration > 0
                            ? math.min(desired, math.max(0, duration - 1))
                            : desired) *
                        1000)
                    .round(),
          ),
        );
      }
      await kernel.setRate(rate);
      await kernel.player.setVolume(_volume);
      if (kIsWeb && _subtitle != null) {
        await adaptiveAction(
          await kernel.player.handle,
          'subtitle',
          _subtitle!,
        );
      }
      if (!mounted || request != _request) return;
      setState(() {
        _loading = false;
      });
      if (play) {
        try {
          await kernel.play();
        } catch (_) {
          /* Browser may require a gesture. */
        }
      }
      if (kIsWeb) {
        final info = await adaptiveInfo(await kernel.player.handle);
        if (mounted && request == _request) setState(() => _info = info);
      }
      if (!mounted || request != _request) return;
      final delay =
          ticket.expiresAt - DateTime.now().millisecondsSinceEpoch - 45000;
      _renew = Timer(
        Duration(milliseconds: delay.clamp(15000, 240000)),
        () => unawaited(_open(preserve: true)),
      );
    } catch (error) {
      _fail(request, error);
    }
  }

  Future<void> _action(String action, [String value = '']) async {
    final kernel = _kernel;
    if (kernel == null) return;
    try {
      await adaptiveAction(await kernel.player.handle, action, value);
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              action == 'pip'
                  ? '当前浏览器无法启用画中画，请先播放视频。'
                  : action == 'screenshot'
                  ? '当前视频无法截图，请等待画面加载后再试。'
                  : '该操作暂不可用，请重试。',
            ),
          ),
        );
      }
    }
  }

  Future<void> _loadSubtitle() async {
    final file = await openFile(
      acceptedTypeGroups: const [
        XTypeGroup(label: '字幕', extensions: ['srt', 'vtt']),
      ],
    );
    if (file == null || !mounted) return;
    if (await file.length() > 2 * 1024 * 1024) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(const SnackBar(content: Text('请选择小于 2 MB 的字幕文件。')));
      }
      return;
    }
    try {
      final content = await file.readAsString();
      if (!mounted) return;
      if (kIsWeb) {
        await _action('subtitle', content);
      } else {
        await _kernel?.loadSubtitle(file.path, title: file.name);
      }
      if (mounted) setState(() => _subtitle = content);
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('请使用 UTF-8 编码的 SRT 或 VTT 字幕。')),
        );
      }
    }
  }

  Future<void> _seek(int delta) async {
    final kernel = _kernel;
    if (kernel == null) return;
    final max = _duration > 0 ? _duration : double.infinity;
    await kernel.seek(
      Duration(
        milliseconds: ((positionSeconds + delta).clamp(0, max) * 1000).round(),
      ),
    );
  }

  Widget _tool(String label, IconData icon, VoidCallback? action) =>
      TextButton.icon(
        onPressed: _loading || _kernel == null ? null : action,
        icon: Icon(icon, size: 17),
        label: Text(label, style: const TextStyle(fontSize: 12)),
      );

  @override
  Widget build(BuildContext context) {
    final enabled = !_loading && _kernel != null;
    final qualities = _info['qualities'] as List? ?? [];
    final audio = _info['audio'] as List? ?? [];
    final skin = MaterialDesktopVideoControlsThemeData(
      visibleOnMount: true,
      playAndPauseOnTap: true,
      seekBarPositionColor: Colors.white,
      seekBarThumbColor: Colors.white,
      bottomButtonBar: [
        const MaterialDesktopPlayOrPauseButton(),
        if (widget.onNext != null)
          IconButton(
            tooltip: '下一集',
            onPressed: widget.onNext,
            icon: const Icon(Icons.skip_next_rounded, color: Colors.white),
          ),
        const MaterialDesktopVolumeButton(),
        const MaterialDesktopPositionIndicator(),
        const Spacer(),
        PopupMenuButton<double>(
          tooltip: '播放速度',
          initialValue: _rate,
          onSelected: (value) async {
            _rate = value;
            await _kernel?.setRate(value);
            if (mounted) setState(() {});
            _report();
          },
          itemBuilder: (_) => [
            for (final value in [.5, .75, 1.0, 1.25, 1.5, 1.75, 2.0, 3.0])
              PopupMenuItem(value: value, child: Text('$value×')),
          ],
          child: Padding(
            padding: const EdgeInsets.all(10),
            child: Text('$_rate×', style: const TextStyle(color: Colors.white)),
          ),
        ),
        const MaterialDesktopFullscreenButton(),
      ],
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(14),
          child: SizedBox(
            height: widget.height,
            child: ColoredBox(
              color: Colors.black,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  if (_kernel != null)
                    MaterialDesktopVideoControlsTheme(
                      normal: skin,
                      fullscreen: skin,
                      child: Video(
                        controller: _kernel!.videoController,
                        fit: _fit,
                        controls: MediaQuery.sizeOf(context).width >= 700
                            ? MaterialDesktopVideoControls
                            : MaterialVideoControls,
                      ),
                    ),
                  if (_loading)
                    const IgnorePointer(
                      child: Center(
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            CircularProgressIndicator(
                              color: Colors.white,
                              strokeWidth: 2,
                            ),
                            SizedBox(height: 16),
                            Text(
                              '正在准备播放…',
                              style: TextStyle(
                                color: Colors.white,
                                fontSize: 13,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  if (_error != null)
                    ColoredBox(
                      color: Colors.black87,
                      child: Center(
                        child: Padding(
                          padding: const EdgeInsets.all(24),
                          child: Column(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const Icon(
                                Icons.wifi_off_rounded,
                                color: Colors.white70,
                                size: 28,
                              ),
                              const SizedBox(height: 14),
                              Text(
                                _error!,
                                textAlign: TextAlign.center,
                                style: const TextStyle(color: Colors.white),
                              ),
                              const SizedBox(height: 16),
                              FilledButton.icon(
                                onPressed: () => _open(preserve: true),
                                icon: const Icon(Icons.refresh_rounded),
                                label: const Text('重试播放'),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(height: 8),
        Wrap(
          spacing: 3,
          runSpacing: 2,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            _tool('后退 10 秒', Icons.replay_10_rounded, () => _seek(-10)),
            _tool('快进 10 秒', Icons.forward_10_rounded, () => _seek(10)),
            if (widget.onPrevious != null)
              _tool('上一集', Icons.skip_previous_rounded, widget.onPrevious),
            if (widget.onNext != null)
              _tool('下一集', Icons.skip_next_rounded, widget.onNext),
            PopupMenuButton<double>(
              tooltip: '倍速',
              enabled: enabled,
              onSelected: (value) async {
                _rate = value;
                await _kernel?.setRate(value);
                if (mounted) setState(() {});
                _report();
              },
              itemBuilder: (_) => [
                for (final value in [.5, .75, 1.0, 1.25, 1.5, 2.0, 3.0])
                  PopupMenuItem(value: value, child: Text('$value×')),
              ],
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Text('倍速 $_rate×', style: const TextStyle(fontSize: 12)),
              ),
            ),
            if (qualities.length > 1)
              PopupMenuButton<String>(
                tooltip: '切换画质',
                enabled: enabled,
                onSelected: (value) async {
                  await _action('quality', value);
                  if (mounted) setState(() => _quality = value);
                },
                itemBuilder: (_) => [
                  const PopupMenuItem(value: 'auto', child: Text('自动画质')),
                  for (final q in qualities)
                    PopupMenuItem(
                      value: '${q['id']}',
                      child: Text(q['label'] as String),
                    ),
                ],
                child: Padding(
                  padding: const EdgeInsets.all(12),
                  child: Text(
                    _quality == 'auto'
                        ? '画质 · 自动'
                        : '画质 · ${qualities.firstWhere((q) => '${q['id']}' == _quality)['label']}',
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
              ),
            if (audio.length > 1)
              PopupMenuButton<String>(
                tooltip: '音轨',
                enabled: enabled,
                icon: const Icon(Icons.multitrack_audio_rounded, size: 18),
                onSelected: (value) => _action('audio', value),
                itemBuilder: (_) => [
                  for (final language in audio)
                    PopupMenuItem(
                      value: language as String,
                      child: Text(language),
                    ),
                ],
              ),
            PopupMenuButton<BoxFit>(
              tooltip: '画面比例',
              enabled: enabled,
              icon: const Icon(Icons.aspect_ratio_rounded, size: 18),
              onSelected: (value) => setState(() => _fit = value),
              itemBuilder: (_) => const [
                PopupMenuItem(value: BoxFit.contain, child: Text('原始比例')),
                PopupMenuItem(value: BoxFit.cover, child: Text('填满画面')),
                PopupMenuItem(value: BoxFit.fill, child: Text('拉伸画面')),
              ],
            ),
            _tool('字幕', Icons.subtitles_outlined, _loadSubtitle),
            if (_subtitle != null)
              _tool('关闭字幕', Icons.subtitles_off_outlined, () async {
                if (kIsWeb) {
                  await _action('subtitle');
                } else {
                  await _kernel?.disableSubtitles();
                }
                if (mounted) setState(() => _subtitle = null);
              }),
            if (_info['pip'] == true)
              _tool(
                '画中画',
                Icons.picture_in_picture_alt_rounded,
                () => _action('pip'),
              ),
            if (kIsWeb)
              _tool(
                '截图',
                Icons.photo_camera_outlined,
                () => _action('screenshot'),
              ),
          ],
        ),
      ],
    );
  }
}
