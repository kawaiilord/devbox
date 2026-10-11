import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sameframe_client/src/player/room_timeline.dart';

Widget timeline(List<Duration> seeks, {int version = 1, bool canSeek = true}) =>
    MaterialApp(
      home: Scaffold(
        body: Center(
          child: SizedBox(
            width: 300,
            child: RoomTimeline(
              positionStream: const Stream.empty(),
              durationStream: const Stream.empty(),
              position: Duration.zero,
              duration: const Duration(seconds: 100),
              sourceVersion: version,
              canSeek: canSeek,
              onSeek: seeks.add,
            ),
          ),
        ),
      ),
    );

void main() {
  testWidgets('drag previews locally and submits one seek on release', (
    tester,
  ) async {
    final seeks = <Duration>[];
    await tester.pumpWidget(timeline(seeks));
    final start = tester.getTopLeft(find.byType(Slider)) + const Offset(28, 24);
    final gesture = await tester.startGesture(start);
    await gesture.moveBy(const Offset(100, 0));
    await tester.pump();
    expect(seeks, isEmpty);
    expect(tester.widget<Slider>(find.byType(Slider)).value, greaterThan(0));
    await gesture.up();
    await tester.pump();
    expect(seeks, hasLength(1));
    expect(seeks.single.inSeconds, greaterThan(20));
  });

  testWidgets('a source switch cancels an in-flight seek', (tester) async {
    final seeks = <Duration>[];
    await tester.pumpWidget(timeline(seeks));
    final gesture = await tester.startGesture(
      tester.getTopLeft(find.byType(Slider)) + const Offset(28, 24),
    );
    await gesture.moveBy(const Offset(90, 0));
    await tester.pump();
    await tester.pumpWidget(timeline(seeks, version: 2));
    await gesture.moveBy(const Offset(20, 0));
    await gesture.up();
    await tester.pump();
    expect(seeks, isEmpty);
  });

  testWidgets('watch-only members cannot seek', (tester) async {
    final seeks = <Duration>[];
    await tester.pumpWidget(timeline(seeks, canSeek: false));
    await tester.drag(find.byType(Slider), const Offset(90, 0));
    expect(seeks, isEmpty);
  });
}
