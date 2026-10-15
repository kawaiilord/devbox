// The owner freezes the shared clock at the actual frame while preparing media.
// Manual controls, another controller, and source changes cancel auto-resume.
class BufferingGate {
  int? _source;
  DateTime? _since;
  bool _waiting = false;
  void reset() {
    _source = null;
    _since = null;
    _waiting = false;
  }

  List<Map<String, dynamic>> observe({
    required int sourceVersion,
    required bool playing,
    required bool ready,
    required double position,
    required DateTime now,
  }) {
    if (_source != sourceVersion) {
      reset();
      _source = sourceVersion;
    }
    if (_waiting) {
      if (ready && !playing) {
        _waiting = false;
        _since = null;
        return [
          {'action': 'play'},
        ];
      }
      return [];
    }
    if (!playing || ready) {
      _since = null;
      return [];
    }
    _since ??= now;
    if (now.difference(_since!) < const Duration(seconds: 1)) return [];
    _waiting = true;
    return [
      {'action': 'pause'},
      {'action': 'seek', 'position': position.clamp(0, double.infinity)},
    ];
  }
}
