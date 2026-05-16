package bpmn_engine

import (
	"testing"
	"time"

	"github.com/corbym/gocrest/is"
	"github.com/corbym/gocrest/then"
)

func Test_boundary_timer_interrupts_task_on_timeout(t *testing.T) {
	bpmnEngine := New()
	process, err := bpmnEngine.LoadFromFile("../../test-cases/boundary-timer-interrupting.bpmn")
	then.AssertThat(t, err, is.Nil())

	// No handler registered — task will stay Active and the boundary timer will fire.
	instance, err := bpmnEngine.CreateAndRunInstance(process.ProcessKey, nil)
	then.AssertThat(t, err, is.Nil())
	// Task is active, boundary timer created but not yet due.
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Active))

	// Wait for the timer to become due.
	time.Sleep(1100 * time.Millisecond)

	_, err = bpmnEngine.RunOrContinueInstance(instance.InstanceKey)
	then.AssertThat(t, err, is.Nil())

	// Interrupting boundary timer should complete the instance via the timeout path.
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Completed))
}

func Test_boundary_timer_does_not_fire_when_task_completes_first(t *testing.T) {
	bpmnEngine := New()
	process, err := bpmnEngine.LoadFromFile("../../test-cases/boundary-timer-interrupting.bpmn")
	then.AssertThat(t, err, is.Nil())

	// Handler completes the task immediately.
	cp := CallPath{}
	bpmnEngine.NewTaskHandler().Id("user-task").Handler(cp.TaskHandler)

	instance, err := bpmnEngine.CreateAndRunInstance(process.ProcessKey, nil)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Completed))
	then.AssertThat(t, cp.CallPath, is.EqualTo("user-task"))

	// Wait past the boundary timer duration; a second run should not re-fire.
	time.Sleep(1100 * time.Millisecond)
	_, err = bpmnEngine.RunOrContinueInstance(instance.InstanceKey)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Completed))
}

func Test_non_interrupting_boundary_timer_fires_side_effect(t *testing.T) {
	bpmnEngine := New()
	process, err := bpmnEngine.LoadFromFile("../../test-cases/boundary-timer-non-interrupting.bpmn")
	then.AssertThat(t, err, is.Nil())

	reminderCalled := false
	bpmnEngine.NewTaskHandler().Id("ReminderTask").Handler(func(job ActivatedJob) {
		reminderCalled = true
		job.Complete()
	})

	// No handler for "user-task" → it stays Active.
	instance, err := bpmnEngine.CreateAndRunInstance(process.ProcessKey, nil)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Active))

	// Wait for the boundary timer.
	time.Sleep(1100 * time.Millisecond)

	_, err = bpmnEngine.RunOrContinueInstance(instance.InstanceKey)
	then.AssertThat(t, err, is.Nil())

	// The reminder task should have been called.
	then.AssertThat(t, reminderCalled, is.True())
	// Instance is still Active because the user task is still pending.
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Active))
}
