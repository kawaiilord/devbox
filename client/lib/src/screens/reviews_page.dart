import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';

class ReviewsPage extends StatefulWidget {
  const ReviewsPage({
    super.key,
    required this.api,
    required this.session,
    required this.targetType,
    required this.targetId,
    required this.title,
  });

  final ApiClient api;
  final Session session;
  final String targetType;
  final String targetId;
  final String title;

  @override
  State<ReviewsPage> createState() => _ReviewsPageState();
}

class _ReviewsPageState extends State<ReviewsPage> {
  final _content = TextEditingController();
  int _rating = 8;
  bool _busy = false;
  String? _error;
  List<Review> _reviews = const [];
  final List<XFile> _images = [];

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _content.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final reviews = await widget.api.reviews(
        widget.session,
        widget.targetType,
        widget.targetId,
      );
      if (mounted) setState(() => _reviews = reviews);
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    }
  }

  Future<void> _pickImages() async {
    final files = await openFiles(
      acceptedTypeGroups: const [
        XTypeGroup(label: '影评图片', extensions: ['jpg', 'jpeg', 'png', 'webp']),
      ],
    );
    if (!mounted) return;
    final available = 4 - _images.length;
    setState(() => _images.addAll(files.take(available)));
  }

  String? _contentType(String name) {
    final lower = name.toLowerCase();
    if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'image/jpeg';
    if (lower.endsWith('.png')) return 'image/png';
    if (lower.endsWith('.webp')) return 'image/webp';
    return null;
  }

  Future<void> _publish() async {
    final content = _content.text.trim();
    if (content.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final keys = <String>[];
      for (final file in _images) {
        final bytes = await file.readAsBytes();
        if (bytes.length > 10 * 1024 * 1024) {
          throw const ApiException('每张图片不能超过 10 MiB');
        }
        final contentType = _contentType(file.name);
        if (contentType == null) throw const ApiException('图片格式不受支持');
        keys.add(
          await widget.api.uploadReviewImage(
            session: widget.session,
            filename: file.name,
            contentType: contentType,
            bytes: bytes,
          ),
        );
      }
      await widget.api.saveReview(
        session: widget.session,
        targetType: widget.targetType,
        targetId: widget.targetId,
        title: widget.title,
        rating: _rating,
        content: content,
        imageKeys: keys,
      );
      _content.clear();
      _images.clear();
      await _load();
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _openComments(Review review) => showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    builder: (_) => _CommentsSheet(
      api: widget.api,
      session: widget.session,
      review: review,
    ),
  );

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('${widget.title} · 影评')),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            TextField(
              controller: _content,
              minLines: 3,
              maxLines: 8,
              maxLength: 5000,
              decoration: const InputDecoration(labelText: '写下你的影评'),
            ),
            Row(
              children: [
                const Text('评分'),
                Expanded(
                  child: Slider(
                    value: _rating.toDouble(),
                    min: 1,
                    max: 10,
                    divisions: 9,
                    label: '$_rating',
                    onChanged: _busy
                        ? null
                        : (value) => setState(() => _rating = value.round()),
                  ),
                ),
                Text('$_rating / 10'),
              ],
            ),
            Wrap(
              spacing: 8,
              children: [
                OutlinedButton.icon(
                  onPressed: _busy || _images.length >= 4 ? null : _pickImages,
                  icon: const Icon(Icons.add_photo_alternate_outlined),
                  label: Text('图片 ${_images.length}/4'),
                ),
                FilledButton(
                  onPressed: _busy ? null : _publish,
                  child: Text(_busy ? '发布中…' : '发布 / 更新'),
                ),
              ],
            ),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.only(top: 12),
                child: Text(
                  _error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            const Divider(height: 32),
            if (_reviews.isEmpty) const Center(child: Text('还没有影评。')),
            for (final review in _reviews)
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(14),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '${review.author.displayName}  ·  ${review.rating}/10',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                      const SizedBox(height: 8),
                      Text(review.content),
                      if (review.imageUrls.isNotEmpty) ...[
                        const SizedBox(height: 10),
                        SizedBox(
                          height: 120,
                          child: ListView.separated(
                            scrollDirection: Axis.horizontal,
                            itemCount: review.imageUrls.length,
                            separatorBuilder: (_, _) =>
                                const SizedBox(width: 8),
                            itemBuilder: (_, index) => ClipRRect(
                              borderRadius: BorderRadius.circular(8),
                              child: Image.network(
                                review.imageUrls[index],
                                width: 160,
                                fit: BoxFit.cover,
                              ),
                            ),
                          ),
                        ),
                      ],
                      TextButton.icon(
                        onPressed: () => _openComments(review),
                        icon: const Icon(Icons.comment_outlined),
                        label: const Text('评论'),
                      ),
                    ],
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _CommentsSheet extends StatefulWidget {
  const _CommentsSheet({
    required this.api,
    required this.session,
    required this.review,
  });
  final ApiClient api;
  final Session session;
  final Review review;
  @override
  State<_CommentsSheet> createState() => _CommentsSheetState();
}

class _CommentsSheetState extends State<_CommentsSheet> {
  final _body = TextEditingController();
  List<ReviewComment> _comments = const [];

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _body.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    final comments = await widget.api.reviewComments(
      widget.session,
      widget.review.id,
    );
    if (mounted) setState(() => _comments = comments);
  }

  Future<void> _send() async {
    final body = _body.text.trim();
    if (body.isEmpty) return;
    await widget.api.addReviewComment(widget.session, widget.review.id, body);
    _body.clear();
    await _load();
  }

  @override
  Widget build(BuildContext context) => Padding(
    padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
    child: SizedBox(
      height: MediaQuery.sizeOf(context).height * .7,
      child: Column(
        children: [
          const Padding(padding: EdgeInsets.all(16), child: Text('影评评论')),
          Expanded(
            child: ListView.builder(
              itemCount: _comments.length,
              itemBuilder: (_, index) {
                final comment = _comments[index];
                return ListTile(
                  title: Text(comment.author.displayName),
                  subtitle: Text(comment.body),
                );
              },
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _body,
                    maxLength: 2000,
                    decoration: const InputDecoration(labelText: '添加评论'),
                  ),
                ),
                IconButton(
                  onPressed: _send,
                  icon: const Icon(Icons.send_rounded),
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}
