import 'dart:async';

import 'package:flutter/material.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../api/api_client.dart';
import '../models.dart';
import '../player/media_player_kernel.dart';

class CatalogPlayer extends StatefulWidget {
  const CatalogPlayer({
    super.key,
    required this.api,
    required this.session,
    required this.selection,
    required this.identity,
    this.onNext,
  });
  final ApiClient api;
  final Session session;
  final Map<String, dynamic> selection;
  final String identity;
  final VoidCallback? onNext;
  @override
  State<CatalogPlayer> createState() => CatalogPlayerState();
}

class CatalogPlayerState extends State<CatalogPlayer> {
  MediaPlayerKernel? _kernel;
  Timer? _renew;
  final List<StreamSubscription<dynamic>> _subscriptions = [];
  bool _loading = true;
  String? _error;
  int _request = 0;
  double get positionSeconds => (_kernel?.position.inMilliseconds ?? 0) / 1000;
  Future<void> pause() async {
    await _kernel?.pause();
  }

  @override
  void initState() {
    super.initState();
    unawaited(_open());
  }

  @override
  void didUpdateWidget(CatalogPlayer oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.identity != widget.identity) {
      unawaited(_open());
    }
  }

  @override
  void dispose() {
    _request++;
    _renew?.cancel();
    for (final subscription in _subscriptions) {
      unawaited(subscription.cancel());
    }
    unawaited(_kernel?.dispose());
    super.dispose();
  }

  Future<void> _open({bool preserve = false}) async {
    final request = ++_request;
    _renew?.cancel();
    final previous = _kernel;
    final position = preserve
        ? previous?.position ?? Duration.zero
        : Duration.zero;
    final play = preserve ? previous?.playing ?? true : true;
    setState(() {
      _kernel = null;
      _loading = true;
      _error = null;
    });
    for (final subscription in _subscriptions) {
      await subscription.cancel();
    }
    _subscriptions.clear();
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
        kernel.errorStream.listen((message) {
          if (mounted && request == _request) {
            setState(() {
              _error = '播放暂时失败，请重试或选择其他线路。';
              _loading = false;
            });
          }
        }),
      );
      _subscriptions.add(
        kernel.player.stream.completed.listen((done) {
          if (done && mounted && request == _request) {
            widget.onNext?.call();
          }
        }),
      );
      await kernel.open(
        ticket.url,
        version: request,
        episodeIndex: 0,
        initialPosition: position,
      );
      if (!mounted || request != _request) return;
      setState(() {
        _loading = false;
        _error = null;
      });
      if (play) {
        try {
          await kernel.play();
        } catch (_) {
          /* Browser can require an explicit play gesture. */
        }
      }
      final delay =
          ticket.expiresAt - DateTime.now().millisecondsSinceEpoch - 45000;
      _renew = Timer(
        Duration(milliseconds: delay.clamp(15000, 240000)),
        () => unawaited(_open(preserve: true)),
      );
    } catch (error) {
      if (mounted && request == _request) {
        setState(() {
          _loading = false;
          _error = error is ApiException ? error.message : '视频暂时无法播放，请重试或切换线路。';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) => ClipRRect(
    borderRadius: BorderRadius.circular(12),
    child: LayoutBuilder(
      builder: (context, box) => SizedBox(
        height: (box.maxWidth * 9 / 16).clamp(220.0, 480.0),
        child: ColoredBox(
          color: Colors.black,
          child: Stack(
            fit: StackFit.expand,
            children: [
              if (_kernel != null)
                Video(
                  controller: _kernel!.videoController,
                  controls: MaterialVideoControls,
                  fit: BoxFit.contain,
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
                          style: TextStyle(color: Colors.white, fontSize: 13),
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
  );
}
