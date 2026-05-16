package bpmn_engine

import (
	"testing"

	"github.com/corbym/gocrest/is"
	"github.com/corbym/gocrest/then"
)

func Test_manual_task_completes_with_handler(t *testing.T) {
	bpmnEngine := New()
	process, err := bpmnEngine.LoadFromFile("../../test-cases/simple-manual-task.bpmn")
	then.AssertThat(t, err, is.Nil())

	cp := CallPath{}
	bpmnEngine.NewTaskHandler().Id("manual-task").Handler(cp.TaskHandler)

	instance, err := bpmnEngine.CreateAndRunInstance(process.ProcessKey, nil)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Completed))
	then.AssertThat(t, cp.CallPath, is.EqualTo("manual-task"))
}

func Test_manual_task_waits_without_handler(t *testing.T) {
	bpmnEngine := New()
	process, err := bpmnEngine.LoadFromFile("../../test-cases/simple-manual-task.bpmn")
	then.AssertThat(t, err, is.Nil())

	instance, err := bpmnEngine.CreateAndRunInstance(process.ProcessKey, nil)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Active))
}

func Test_manual_task_can_be_continued(t *testing.T) {
	bpmnEngine := New()
	process, err := bpmnEngine.LoadFromFile("../../test-cases/simple-manual-task.bpmn")
	then.AssertThat(t, err, is.Nil())

	confirmed := false
	bpmnEngine.NewTaskHandler().Id("manual-task").Handler(func(job ActivatedJob) {
		if confirmed {
			job.Complete()
		}
	})

	instance, err := bpmnEngine.CreateInstance(process.ProcessKey, nil)
	then.AssertThat(t, err, is.Nil())

	_, err = bpmnEngine.RunOrContinueInstance(instance.InstanceKey)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Active))

	confirmed = true
	_, err = bpmnEngine.RunOrContinueInstance(instance.InstanceKey)
	then.AssertThat(t, err, is.Nil())
	then.AssertThat(t, instance.ActivityState, is.EqualTo(Completed))
}
