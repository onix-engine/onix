"use strict";

(() => {
	class OnixWebGPUFrame {
		constructor(context, device, queue, colorAttachment, depthAttachment, descriptor) {
			this.context = context;
			this.device = device;
			this.queue = queue;
			this.colorAttachment = colorAttachment;
			this.depthAttachment = depthAttachment;
			this.descriptor = descriptor;
			this.commands = [null];
			this.encoder = null;
			this.pass = null;
		}

		begin(resolveView, depthView) {
			if (this.pass !== null) throw new Error("WebGPU frame is already active");
			const canvasView = this.context.getCurrentTexture().createView();
			if (resolveView === null) {
				this.colorAttachment.view = canvasView;
				this.colorAttachment.storeOp = "store";
				delete this.colorAttachment.resolveTarget;
			} else {
				this.colorAttachment.view = resolveView;
				this.colorAttachment.resolveTarget = canvasView;
				this.colorAttachment.storeOp = "store";
			}
			this.colorAttachment.loadOp = "clear";
			this.setDepth(depthView, "clear");
			this.encoder = this.device.createCommandEncoder();
			this.pass = this.encoder.beginRenderPass(this.descriptor);
		}

		setDepth(depthView, loadOp) {
			this.depthAttachment.view = depthView;
			this.depthAttachment.depthLoadOp = loadOp;
			if (depthView === null) delete this.descriptor.depthStencilAttachment;
			else this.descriptor.depthStencilAttachment = this.depthAttachment;
		}

		requireDepth(depthView) {
			this.pass.end();
			this.colorAttachment.loadOp = "load";
			this.setDepth(depthView, "clear");
			this.pass = this.encoder.beginRenderPass(this.descriptor);
		}

		beginTarget(descriptor) {
			if (this.pass === null) throw new Error("WebGPU frame is not active");
			this.pass.end();
			this.pass = this.encoder.beginRenderPass(descriptor);
		}

		endTarget() {
			if (this.pass === null) throw new Error("WebGPU target pass is not active");
			this.pass.end();
			this.colorAttachment.loadOp = "load";
			this.depthAttachment.depthLoadOp = "load";
			this.pass = this.encoder.beginRenderPass(this.descriptor);
		}

		present() {
			if (this.pass === null) return;
			this.pass.end();
			this.commands[0] = this.encoder.finish();
			this.queue.submit(this.commands);
			this.commands[0] = null;
			this.release();
		}

		destroy() {
			if (this.pass !== null) this.pass.end();
			this.commands[0] = null;
			this.release();
		}

		release() {
			this.colorAttachment.view = null;
			delete this.colorAttachment.resolveTarget;
			this.depthAttachment.view = null;
			this.encoder = null;
			this.pass = null;
		}

		setViewport(x, y, width, height, minDepth, maxDepth) {
			this.pass.setViewport(x, y, width, height, minDepth, maxDepth);
		}

		setScissorRect(x, y, width, height) {
			this.pass.setScissorRect(x, y, width, height);
		}

		setVertexBuffer(slot, buffer, offset) {
			if (offset === undefined) this.pass.setVertexBuffer(slot, buffer);
			else this.pass.setVertexBuffer(slot, buffer, offset);
		}

		setIndexBuffer(buffer, format, offset) {
			if (offset === undefined) this.pass.setIndexBuffer(buffer, format);
			else this.pass.setIndexBuffer(buffer, format, offset);
		}

		setPipeline(pipeline) {
			this.pass.setPipeline(pipeline);
		}

		setBlendConstant(color) {
			this.pass.setBlendConstant(color);
		}

		setBindGroup(index, bindGroup, dynamicOffsets) {
			this.pass.setBindGroup(index, bindGroup, dynamicOffsets);
		}

		drawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance) {
			this.pass.drawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance);
		}

		setStencilReference(reference) {
			this.pass.setStencilReference(reference);
		}
	}

	globalThis.__onixWebGPUFrame = OnixWebGPUFrame;
})();
