#ifndef ONIX_GLES_UPLOAD_H
#define ONIX_GLES_UPLOAD_H

#include "native_buffer.h"
#include "native_upload.h"

typedef struct {
	GLuint buffer;
	size_t capacity, snapshot_capacity;
	void *snapshot;
} OnixGLUploadBuffer;

typedef struct {
	OnixGLUploadBuffer *buffers;
	uint32_t count, capacity, used;
	uint64_t bytes, snapshot_bytes, demand, snapshot_demand;
	GLsync fence;
	GLuint vaos[3], quad_vertex, quad_index;
	OnixBufferRetention retention, snapshot_retention, table_retention;
	OnixNativeUploadCache cache;
} OnixGLUploadPool;

static void onix_gl_delete_upload_buffer(OnixGLUploadBuffer *buffer) {
	if (buffer->buffer) glDeleteBuffers(1, &buffer->buffer);
	free(buffer->snapshot);
}

static int onix_gl_begin_uploads(OnixGLUploadPool *pool) {
	if (pool->fence) {
		GLenum result;
		do { result = glClientWaitSync(pool->fence, GL_SYNC_FLUSH_COMMANDS_BIT, UINT64_C(1000000000)); }
		while (result == GL_TIMEOUT_EXPIRED);
		if (result == GL_WAIT_FAILED) return 0;
		glDeleteSync(pool->fence);
		pool->fence = NULL;
	}
	onix_upload_cache_reset(&pool->cache);
	uint64_t budget = onix_buffer_trim_capacity(&pool->retention, pool->bytes, pool->demand, 1024);
	uint64_t snapshot_budget = onix_buffer_trim_capacity(&pool->snapshot_retention, pool->snapshot_bytes, pool->snapshot_demand, 1024);
	if (budget) {
		// Other VAOs can retain deleted buffer storage. Detach the transient
		// attributes before trimming, so an idle view also releases its peak.
		if (pool->quad_vertex) {
			glBindBuffer(GL_ARRAY_BUFFER, pool->quad_vertex);
			for (uint32_t index = 0; index < 3; index++) {
				glBindVertexArray(pool->vaos[index]);
				for (GLuint attribute = 0; attribute < 16; attribute++) {
					glVertexAttribPointer(attribute, 4, GL_FLOAT, GL_FALSE, 20, NULL);
				}
				glVertexAttribPointer(0, 3, GL_FLOAT, GL_FALSE, 20, NULL);
				glVertexAttribPointer(1, 2, GL_FLOAT, GL_FALSE, 20, (const void *)12);
				glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, pool->quad_index);
			}
		}
	}
	if (budget || snapshot_budget) {
		uint32_t keep = 0;
		pool->bytes = pool->snapshot_bytes = 0;
		for (uint32_t index = 0; index < pool->count; index++) {
			OnixGLUploadBuffer buffer = pool->buffers[index];
			if (budget && buffer.capacity > budget - pool->bytes) {
				onix_gl_delete_upload_buffer(&buffer);
				continue;
			}
			if (snapshot_budget && buffer.snapshot_capacity > snapshot_budget - pool->snapshot_bytes) {
				free(buffer.snapshot);
				buffer.snapshot = NULL;
				buffer.snapshot_capacity = 0;
			}
			pool->buffers[keep++] = buffer;
			pool->bytes += buffer.capacity;
			pool->snapshot_bytes += buffer.snapshot_capacity;
		}
		pool->count = keep;
	}
	pool->buffers = onix_buffer_trim_array(pool->buffers, &pool->capacity, pool->count,
		sizeof(*pool->buffers), 8, &pool->table_retention);
	pool->used = 0;
	pool->demand = pool->snapshot_demand = 0;
	glBindVertexArray(0);
	glBindBuffer(GL_ARRAY_BUFFER, 0);
	return 1;
}

static int onix_gl_finish_uploads(OnixGLUploadPool *pool) {
	if (pool->fence) glDeleteSync(pool->fence);
	pool->fence = glFenceSync(GL_SYNC_GPU_COMMANDS_COMPLETE, 0);
	return pool->fence != NULL;
}

static void onix_gl_destroy_uploads(OnixGLUploadPool *pool) {
	if (pool->fence) glDeleteSync(pool->fence);
	for (uint32_t index = 0; index < pool->count; index++) onix_gl_delete_upload_buffer(&pool->buffers[index]);
	free(pool->buffers);
	onix_upload_cache_destroy(&pool->cache);
	*pool = (OnixGLUploadPool){0};
}

// Each block is written once between fences. Reusing buffer names alone is
// insufficient: overwriting an earlier batch's range would stall the GPU.
static GLuint onix_gl_upload_buffer(OnixGLUploadPool *pool, GLenum target, size_t size, const void *data, int cached) {
	OnixNativeUpload upload;
	uint64_t hash = cached ? onix_upload_hash(data, size) : 0;
	if (cached && onix_upload_cache_find(&pool->cache, data, size, hash, 1, &upload)) return (GLuint)upload.resource;
	if (pool->used == pool->count) {
		if (pool->count == pool->capacity) {
			uint32_t capacity = pool->capacity ? pool->capacity * 2 : 8;
			if (capacity < pool->capacity || (uint64_t)capacity * sizeof(*pool->buffers) > SIZE_MAX) return 0;
			OnixGLUploadBuffer *buffers = realloc(pool->buffers, (size_t)capacity * sizeof(*buffers));
			if (!buffers) return 0;
			pool->buffers = buffers;
			pool->capacity = capacity;
		}
		pool->buffers[pool->count++] = (OnixGLUploadBuffer){0};
	}
	OnixGLUploadBuffer *buffer = &pool->buffers[pool->used];
	if (!buffer->buffer) glGenBuffers(1, &buffer->buffer);
	if (!buffer->buffer) return 0;
	glBindBuffer(target, buffer->buffer);
	if (size > buffer->capacity) {
		size_t capacity = 1024;
		while (capacity < size && capacity <= SIZE_MAX / 2) capacity *= 2;
		if (capacity < size) return 0;
		glBufferData(target, (GLsizeiptr)capacity, NULL, GL_DYNAMIC_DRAW);
		pool->bytes += capacity - buffer->capacity;
		buffer->capacity = capacity;
	}
	if (cached && size > buffer->snapshot_capacity) {
		void *snapshot = realloc(buffer->snapshot, size);
		if (!snapshot) return 0;
		pool->snapshot_bytes += size - buffer->snapshot_capacity;
		buffer->snapshot = snapshot;
		buffer->snapshot_capacity = size;
	}
	glBufferSubData(target, 0, (GLsizeiptr)size, data);
	pool->used++;
	pool->demand += size;
	if (cached) {
		pool->snapshot_demand += size;
		memcpy(buffer->snapshot, data, size);
		upload = (OnixNativeUpload){.resource = buffer->buffer, .hash = hash, .size = size, .data = buffer->snapshot};
		if (!onix_upload_cache_insert(&pool->cache, upload)) return 0;
	}
	return buffer->buffer;
}

#endif
