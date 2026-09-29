# Context bundle — CVE-2023-52835 backport

Paste this whole file into the chat, right after the prompt. Both prompt
versions get exactly this bundle, unchanged.

## Target

- Kernel: `kernel-4.18.0-448.el8` (CentOS Stream 8, RHEL 8 based), x86_64
- Toolchain: gcc 8.5.0 (`gcc-8.5.0-22.el8`), binutils 2.30
- Relevant config: `CONFIG_PERF_EVENTS=y`
- The fix will be shipped as a `PatchNNNN:` file in the kernel SRPM and
  applied with `patch -p1` on top of the unpacked source tree.

## Upstream fix (mainline), as published on git.kernel.org

```
From 54aee5f15b83437f23b2b2469bcf21bdd9823916 Mon Sep 17 00:00:00 2001
From: Shuai Xue <xueshuai@linux.alibaba.com>
Date: Thu, 7 Sep 2023 08:43:07 +0800
Subject: perf/core: Bail out early if the request AUX area is out of bound

When perf-record with a large AUX area, e.g 4GB, it fails with:

    #perf record -C 0 -m ,4G -e arm_spe_0// -- sleep 1
    failed to mmap with 12 (Cannot allocate memory)

and it reveals a WARNING with __alloc_pages():

	------------[ cut here ]------------
	WARNING: CPU: 44 PID: 17573 at mm/page_alloc.c:5568 __alloc_pages+0x1ec/0x248
	Call trace:
	 __alloc_pages+0x1ec/0x248
	 __kmalloc_large_node+0xc0/0x1f8
	 __kmalloc_node+0x134/0x1e8
	 rb_alloc_aux+0xe0/0x298
	 perf_mmap+0x440/0x660
	 mmap_region+0x308/0x8a8
	 do_mmap+0x3c0/0x528
	 vm_mmap_pgoff+0xf4/0x1b8
	 ksys_mmap_pgoff+0x18c/0x218
	 __arm64_sys_mmap+0x38/0x58
	 invoke_syscall+0x50/0x128
	 el0_svc_common.constprop.0+0x58/0x188
	 do_el0_svc+0x34/0x50
	 el0_svc+0x34/0x108
	 el0t_64_sync_handler+0xb8/0xc0
	 el0t_64_sync+0x1a4/0x1a8

'rb->aux_pages' allocated by kcalloc() is a pointer array which is used to
maintains AUX trace pages. The allocated page for this array is physically
contiguous (and virtually contiguous) with an order of 0..MAX_ORDER. If the
size of pointer array crosses the limitation set by MAX_ORDER, it reveals a
WARNING.

So bail out early with -ENOMEM if the request AUX area is out of bound,
e.g.:

    #perf record -C 0 -m ,4G -e arm_spe_0// -- sleep 1
    failed to mmap with 12 (Cannot allocate memory)

Signed-off-by: Shuai Xue <xueshuai@linux.alibaba.com>
Signed-off-by: Peter Zijlstra (Intel) <peterz@infradead.org>
Signed-off-by: Ingo Molnar <mingo@kernel.org>
---
 kernel/events/ring_buffer.c | 6 ++++++
 1 file changed, 6 insertions(+)

diff --git a/kernel/events/ring_buffer.c b/kernel/events/ring_buffer.c
index fb1e180b5f0af..e8d82c2f07d0e 100644
--- a/kernel/events/ring_buffer.c
+++ b/kernel/events/ring_buffer.c
@@ -700,6 +700,12 @@ int rb_alloc_aux(struct perf_buffer *rb, struct perf_event *event,
 		watermark = 0;
 	}
 
+	/*
+	 * kcalloc_node() is unable to allocate buffer if the size is larger
+	 * than: PAGE_SIZE << MAX_ORDER; directly bail out in this case.
+	 */
+	if (get_order((unsigned long)nr_pages * sizeof(void *)) > MAX_ORDER)
+		return -ENOMEM;
 	rb->aux_pages = kcalloc_node(nr_pages, sizeof(void *), GFP_KERNEL,
 				     node);
 	if (!rb->aux_pages)
```

## Target code: `rb_alloc_aux()` in `kernel/events/ring_buffer.c` of 4.18.0-448.el8 (unpatched)

Line numbers are those of the target file.

```c
 667	int rb_alloc_aux(struct perf_buffer *rb, struct perf_event *event,
 668			 pgoff_t pgoff, int nr_pages, long watermark, int flags)
 669	{
 670		bool overwrite = !(flags & RING_BUFFER_WRITABLE);
 671		int node = (event->cpu == -1) ? -1 : cpu_to_node(event->cpu);
 672		int ret = -ENOMEM, max_order;
 673	
 674		if (!has_aux(event))
 675			return -EOPNOTSUPP;
 676	
 677		if (!overwrite) {
 678			/*
 679			 * Watermark defaults to half the buffer, and so does the
 680			 * max_order, to aid PMU drivers in double buffering.
 681			 */
 682			if (!watermark)
 683				watermark = nr_pages << (PAGE_SHIFT - 1);
 684	
 685			/*
 686			 * Use aux_watermark as the basis for chunking to
 687			 * help PMU drivers honor the watermark.
 688			 */
 689			max_order = get_order(watermark);
 690		} else {
 691			/*
 692			 * We need to start with the max_order that fits in nr_pages,
 693			 * not the other way around, hence ilog2() and not get_order.
 694			 */
 695			max_order = ilog2(nr_pages);
 696			watermark = 0;
 697		}
 698	
 699		rb->aux_pages = kcalloc_node(nr_pages, sizeof(void *), GFP_KERNEL,
 700					     node);
 701		if (!rb->aux_pages)
 702			return -ENOMEM;
 703	
 704		rb->free_aux = event->pmu->free_aux;
 705		for (rb->aux_nr_pages = 0; rb->aux_nr_pages < nr_pages;) {
 706			struct page *page;
 707			int last, order;
 708	
 709			order = min(max_order, ilog2(nr_pages - rb->aux_nr_pages));
 710			page = rb_alloc_aux_page(node, order);
 711			if (!page)
 712				goto out;
 713	
 714			for (last = rb->aux_nr_pages + (1 << page_private(page));
 715			     last > rb->aux_nr_pages; rb->aux_nr_pages++)
 716				rb->aux_pages[rb->aux_nr_pages] = page_address(page++);
 717		}
 718	
 719		/*
 720		 * In overwrite mode, PMUs that don't support SG may not handle more
 721		 * than one contiguous allocation, since they rely on PMI to do double
 722		 * buffering. In this case, the entire buffer has to be one contiguous
 723		 * chunk.
 724		 */
 725		if ((event->pmu->capabilities & PERF_PMU_CAP_AUX_NO_SG) &&
 726		    overwrite) {
 727			struct page *page = virt_to_page(rb->aux_pages[0]);
 728	
 729			if (page_private(page) != max_order)
 730				goto out;
 731		}
 732	
 733		rb->aux_priv = event->pmu->setup_aux(event, rb->aux_pages, nr_pages,
 734						     overwrite);
 735		if (!rb->aux_priv)
 736			goto out;
 737	
 738		ret = 0;
 739	
 740		/*
 741		 * aux_pages (and pmu driver's private data, aux_priv) will be
 742		 * referenced in both producer's and consumer's contexts, thus
 743		 * we keep a refcount here to make sure either of the two can
 744		 * reference them safely.
 745		 */
 746		refcount_set(&rb->aux_refcount, 1);
 747	
 748		rb->aux_overwrite = overwrite;
 749		rb->aux_watermark = watermark;
 750	
 751	out:
 752		if (!ret)
 753			rb->aux_pgoff = pgoff;
 754		else
 755			__rb_free_aux(rb);
 756	
 757		return ret;
 758	}
```
