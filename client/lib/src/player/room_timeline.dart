import 'dart:math' as math;

import 'package:flutter/material.dart';

/// Keeps local drag feedback separate from the shared room's seek command.
class RoomTimeline extends StatefulWidget {
  const RoomTimeline({
    super.key,
    required this.positionStream,
    required this.durationStream,
    required this.position,
    required this.duration,
    required this.sourceVersion,
    required this.canSeek,
    required this.onSeek,
  });
  final Stream<Duration> positionStream, durationStream;
  final Duration position, duration;
  final int sourceVersion;
  final bool canSeek;
  final ValueChanged<Duration> onSeek;

  @override
  State<RoomTimeline> createState() => _RoomTimelineState();
}

class _RoomTimelineState extends State<RoomTimeline> {
  double? _dragPosition;
  int? _dragVersion;
  bool _dragging = false;

  @override
  void didUpdateWidget(covariant RoomTimeline oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!widget.canSeek || oldWidget.sourceVersion != widget.sourceVersion) {
      _dragPosition = null;
      _dragVersion = null;
      if (!widget.canSeek) _dragging = false;
    }
  }

  void _start(double value) => setState(() {
    _dragging = true;
    _dragVersion = widget.sourceVersion;
    _dragPosition = value;
  });

  void _change(double value) {
    if (!widget.canSeek) return;
    if (!_dragging) {
      // Keyboard and accessibility adjustments can arrive without a drag.
      widget.onSeek(Duration(milliseconds: value.round()));
    } else if (_dragVersion == widget.sourceVersion) {
      setState(() => _dragPosition = value);
    }
  }

  void _end(double value) {
    final valid =
        _dragging && _dragVersion == widget.sourceVersion && widget.canSeek;
    setState(() {
      _dragging = false;
      _dragVersion = null;
      _dragPosition = null;
    });
    if (valid) widget.onSeek(Duration(milliseconds: value.round()));
  }

  @override
  Widget build(BuildContext context) => StreamBuilder<Duration>(
    stream: widget.durationStream,
    initialData: widget.duration,
    builder: (context, durationSnapshot) => StreamBuilder<Duration>(
      stream: widget.positionStream,
      initialData: widget.position,
      builder: (context, positionSnapshot) {
        final duration = durationSnapshot.data ?? Duration.zero;
        final position = positionSnapshot.data ?? Duration.zero;
        final maximum = math.max(1, duration.inMilliseconds).toDouble();
        final value = (_dragPosition ?? position.inMilliseconds.toDouble())
            .clamp(0.0, maximum);
        return Column(
          children: [
            Slider(
              value: value,
              max: maximum,
              onChangeStart: widget.canSeek ? _start : null,
              onChanged: widget.canSeek ? _change : null,
              onChangeEnd: widget.canSeek ? _end : null,
            ),
            Row(
              children: [
                Text(
                  _format(
                    _dragPosition == null
                        ? position
                        : Duration(milliseconds: value.round()),
                  ),
                ),
                const Spacer(),
                Text(_format(duration)),
              ],
            ),
          ],
        );
      },
    ),
  );

  String _format(Duration value) {
    final minutes = value.inMinutes.remainder(60).toString().padLeft(2, '0');
    final seconds = value.inSeconds.remainder(60).toString().padLeft(2, '0');
    return value.inHours > 0
        ? '${value.inHours}:$minutes:$seconds'
        : '$minutes:$seconds';
  }
}
